package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"totp_route/pkg/client"
	"totp_route/pkg/config"
	"totp_route/pkg/server"
	"totp_route/pkg/totp"
)

const (
	version = "1.0.0"
	banner  = `
 ████████  ██████  ████████ ██████      ██████   ██████  ██    ██ ████████ ███████ 
    ██    ██    ██    ██    ██   ██     ██   ██ ██    ██ ██    ██    ██    ██      
    ██    ██    ██    ██    ██████      ██████  ██    ██ ██    ██    ██    █████   
    ██    ██    ██    ██    ██          ██   ██ ██    ██ ██    ██    ██    ██      
    ██     ██████     ██    ██          ██   ██  ██████   ██████     ██    ███████ 

    基于TOTP的随机端口流量转发工具 v%s
    Github: https://github.com/your-repo/totp_route
`
)

func main() {
	// 命令行参数
	var (
		configFile = flag.String("c", "config.toml", "配置文件路径")
		showHelp   = flag.Bool("h", false, "显示帮助信息")
		showVer    = flag.Bool("v", false, "显示版本信息")
		testMode   = flag.Bool("t", false, "测试模式（验证配置和连接）")
	)
	flag.Parse()

	// 显示版本信息
	if *showVer {
		fmt.Printf("totp_route v%s\n", version)
		os.Exit(0)
	}

	// 显示帮助信息
	if *showHelp {
		showUsage()
		os.Exit(0)
	}

	// 显示banner
	fmt.Printf(banner, version)
	fmt.Println()

	// 加载配置
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	log.Printf("配置加载成功: %s", *configFile)
	log.Printf("运行模式: %s", cfg.Mode)
	log.Printf("协议: %s", cfg.Protocol)
	log.Printf("TOTP间隔: %d秒", cfg.Interval)
	log.Printf("时间扩展: %d秒", cfg.Extend)
	log.Printf("端口范围: %d-%d", cfg.BasePort, cfg.BasePort+cfg.PortRange-1)

	// 测试模式
	if *testMode {
		runTestMode(cfg)
		return
	}

	// 根据模式启动相应的服务
	switch cfg.Mode {
	case "server":
		runServer(cfg)
	case "client":
		runClient(cfg)
	default:
		log.Fatalf("无效的运行模式: %s，只支持 'server' 或 'client'", cfg.Mode)
	}
}

// showUsage 显示使用说明
func showUsage() {
	fmt.Printf(`totp_route v%s - 基于TOTP的随机端口流量转发工具

用法:
    totp_route [选项]

选项:
    -c <文件>    指定配置文件路径 (默认: config.toml)
    -t           测试模式，验证配置和连接
    -v           显示版本信息
    -h           显示此帮助信息

配置文件示例 (config.toml):
    interval = 30              # TOTP时间间隔（秒）
    extend = 15                # 时间窗口扩展（秒）
    base_port = 3000           # 基础端口
    port_range = 1000          # 端口范围
    secret = "YOUR_SECRET"     # TOTP密钥
    offsets = [-15, 0, 15]     # 时间偏移量
    host = "127.0.0.1"         # 主机地址
    port = 8080                # 端口
    mode = "server"            # 模式: server 或 client
    protocol = "tcp"           # 协议: tcp 或 udp

运行模式:
    server - 服务端模式：动态监听TOTP端口，转发到目标服务
    client - 客户端模式：本地监听，转发到服务端TOTP端口

示例:
    # 启动服务端
    totp_route -c server.toml

    # 启动客户端
    totp_route -c client.toml

    # 测试配置
    totp_route -t -c config.toml

更多信息请访问: https://github.com/your-repo/totp_route
`, version)
}

// runServer 运行服务端
func runServer(cfg *config.Config) {
	log.Printf("启动服务端模式...")
	log.Printf("目标服务: %s:%d", cfg.Host, cfg.Port)

	// 创建服务端实例
	srv := server.New(cfg)

	// 设置信号处理
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	// 启动服务端协程
	errChan := make(chan error, 1)
	go func() {
		errChan <- srv.Start()
	}()

	// 等待信号或错误
	select {
	case err := <-errChan:
		if err != nil {
			log.Fatalf("服务端启动失败: %v", err)
		}
	case sig := <-signalChan:
		log.Printf("收到信号 %v，正在关闭服务端...", sig)
		srv.Stop()
		log.Println("服务端已关闭")
	}
}

// runClient 运行客户端
func runClient(cfg *config.Config) {
	log.Printf("启动客户端模式...")
	log.Printf("本地监听端口: %d", cfg.Port)
	log.Printf("服务端地址: %s", cfg.Host)

	// 创建客户端实例
	cli := client.New(cfg)

	// 设置信号处理
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	// 启动客户端协程
	errChan := make(chan error, 1)
	go func() {
		errChan <- cli.Start()
	}()

	// 等待信号或错误
	select {
	case err := <-errChan:
		if err != nil {
			log.Fatalf("客户端启动失败: %v", err)
		}
	case sig := <-signalChan:
		log.Printf("收到信号 %v，正在关闭客户端...", sig)
		cli.Stop()
		log.Println("客户端已关闭")
	}
}

// runTestMode 运行测试模式
func runTestMode(cfg *config.Config) {
	log.Println("=== 测试模式 ===")
	
	// 验证配置
	log.Println("1. 验证配置...")
	log.Printf("   模式: %s", cfg.Mode)
	log.Printf("   协议: %s", cfg.Protocol)
	log.Printf("   TOTP密钥: %s", maskSecret(cfg.Secret))
	log.Printf("   基础端口: %d", cfg.BasePort)
	log.Printf("   端口范围: %d", cfg.PortRange)
	log.Printf("   时间间隔: %d秒", cfg.Interval)
	log.Printf("   时间扩展: %d秒", cfg.Extend)
	log.Printf("   偏移量: %v", cfg.Offsets)
	log.Printf("   主机: %s", cfg.Host)
	log.Printf("   端口: %d", cfg.Port)

	// 测试TOTP功能
	log.Println("\n2. 测试TOTP功能...")
	testTOTP(cfg)

	// 根据模式进行特定测试
	switch cfg.Mode {
	case "server":
		log.Println("\n3. 测试服务端功能...")
		testServer(cfg)
	case "client":
		log.Println("\n3. 测试客户端功能...")
		testClient(cfg)
	}

	log.Println("\n=== 测试完成 ===")
}

// testTOTP 测试TOTP功能
func testTOTP(cfg *config.Config) {
	totpGen := NewTOTPFromConfig(cfg)
	
	// 获取当前端口
	currentPort, err := totpGen.GetCurrentPort(cfg.BasePort, cfg.PortRange)
	if err != nil {
		log.Printf("   ❌ 获取当前TOTP端口失败: %v", err)
		return
	}
	log.Printf("   ✓ 当前TOTP端口: %d", currentPort)

	// 获取所有偏移量的端口
	log.Printf("   ✓ 偏移量端口:")
	for _, offset := range cfg.Offsets {
		port, err := totpGen.GetPortWithOffset(offset, cfg.BasePort, cfg.PortRange)
		if err != nil {
			log.Printf("     偏移 %d: 错误 - %v", offset, err)
		} else {
			log.Printf("     偏移 %d: 端口 %d", offset, port)
		}
	}
}

// testServer 测试服务端功能
func testServer(cfg *config.Config) {
	log.Printf("   目标服务: %s:%d", cfg.Host, cfg.Port)
	
	// 尝试连接目标服务
	log.Printf("   测试目标服务连接...")
	if cfg.Protocol == "tcp" {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port), 3*time.Second)
		if err != nil {
			log.Printf("   ❌ 无法连接到目标服务: %v", err)
		} else {
			conn.Close()
			log.Printf("   ✓ 目标服务连接正常")
		}
	} else {
		log.Printf("   ⚠️  UDP协议暂不支持连接测试")
	}
}

// testClient 测试客户端功能
func testClient(cfg *config.Config) {
	log.Printf("   服务端地址: %s", cfg.Host)
	log.Printf("   本地监听端口: %d", cfg.Port)
	
	// 测试服务端连接
	cli := client.New(cfg)
	log.Printf("   测试服务端TOTP端口连接...")
	
	err := cli.TestServerConnection()
	if err != nil {
		log.Printf("   ❌ 服务端连接测试失败: %v", err)
		log.Printf("   💡 请确保服务端正在运行且配置正确")
	} else {
		log.Printf("   ✓ 服务端连接测试成功")
	}
}

// NewTOTPFromConfig 从配置创建TOTP生成器
func NewTOTPFromConfig(cfg *config.Config) *totp.Generator {
	return totp.New(cfg.Secret, cfg.Interval)
}

// maskSecret 掩码显示密钥
func maskSecret(secret string) string {
	if len(secret) <= 4 {
		return "****"
	}
	return secret[:2] + "****" + secret[len(secret)-2:]
}

