package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config 定义应用程序配置结构
type Config struct {
	Interval   int      `mapstructure:"interval"`   // TOTP 时间间隔（秒）
	Extend     int      `mapstructure:"extend"`     // 时间窗口扩展（秒）
	BasePort   int      `mapstructure:"base_port"`  // 基础端口
	PortRange  int      `mapstructure:"port_range"` // 端口范围
	Secret     string   `mapstructure:"secret"`     // TOTP 密钥
	Offsets    []int    `mapstructure:"offsets"`    // 时间偏移量数组
	Host       string   `mapstructure:"host"`       // 主机地址（服务端为目标地址，客户端为服务器地址）
	Port       int      `mapstructure:"port"`       // 端口（服务端为目标端口，客户端为本地监听端口）
	Mode       string   `mapstructure:"mode"`       // 运行模式：server 或 client
	Protocol   string   `mapstructure:"protocol"`   // 协议：tcp 或 udp
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Interval:  30,
		Extend:    15,
		BasePort:  3000,
		PortRange: 1000,
		Secret:    "",
		Offsets:   []int{-15, 0, 15},
		Host:      "127.0.0.1",
		Port:      8080,
		Mode:      "server",
		Protocol:  "tcp",
	}
}

// LoadConfig 从指定文件加载配置
func LoadConfig(configFile string) (*Config, error) {
	// 设置默认配置
	config := DefaultConfig()
	
	// 检查配置文件是否存在
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		// 如果是默认配置文件 config.toml 不存在，尝试从 config.toml.example 复制
		if configFile == "config.toml" {
			if err := copyExampleConfig(); err != nil {
				return nil, fmt.Errorf("配置文件 %s 不存在，且无法创建示例配置文件: %v", configFile, err)
			}
			fmt.Println("初始化程序配置文件 config.toml，请根据需要修改配置参数.")
			return config, nil
		}
		return nil, fmt.Errorf("配置文件 %s 不存在", configFile)
	}

	// 设置 Viper 配置
	viper.SetConfigFile(configFile)
	viper.SetConfigType("toml")

	// 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	// 将配置解析到结构体
	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	// 验证配置
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("配置验证失败: %v", err)
	}

	return config, nil
}

// copyExampleConfig 从 config.toml.example 复制到 config.toml
func copyExampleConfig() error {
	exampleFile := "config.toml.example"
	targetFile := "config.toml"

	// 检查示例文件是否存在
	if _, err := os.Stat(exampleFile); os.IsNotExist(err) {
		return fmt.Errorf("配置文件 %s 不存在", exampleFile)
	}

	// 读取示例文件内容
	data, err := os.ReadFile(exampleFile)
	if err != nil {
		return fmt.Errorf("读取示例配置文件失败: %v", err)
	}

	// 写入到目标文件
	if err := os.WriteFile(targetFile, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %v", err)
	}

	return nil
}

// validateConfig 验证配置的有效性
func validateConfig(config *Config) error {
	if config.Interval <= 0 {
		return fmt.Errorf("interval 必须大于 0")
	}

	if config.Extend < 0 {
		return fmt.Errorf("extend 不能为负数")
	}

	if config.BasePort <= 0 || config.BasePort > 65535 {
		return fmt.Errorf("base_port 必须在 1-65535 范围内")
	}

	if config.PortRange <= 0 {
		return fmt.Errorf("port_range 必须大于 0")
	}

	if config.BasePort+config.PortRange > 65535 {
		return fmt.Errorf("base_port + port_range 不能超过 65535")
	}

	if config.Secret == "" {
		return fmt.Errorf("secret 不能为空")
	}

	if len(config.Offsets) == 0 {
		return fmt.Errorf("offsets 不能为空")
	}

	if config.Host == "" {
		return fmt.Errorf("host 不能为空")
	}

	if config.Port <= 0 || config.Port > 65535 {
		return fmt.Errorf("port 必须在 1-65535 范围内")
	}

	if config.Mode != "server" && config.Mode != "client" {
		return fmt.Errorf("mode 必须是 'server' 或 'client'")
	}

	if config.Protocol != "tcp" && config.Protocol != "udp" {
		return fmt.Errorf("protocol 必须是 'tcp' 或 'udp'")
	}

	return nil
}

// GetConfigDir 获取配置文件目录
func GetConfigDir() (string, error) {
	// 优先使用当前目录
	currentDir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return currentDir, nil
}

// FindConfigFile 在多个路径中查找配置文件
func FindConfigFile(filename string) (string, error) {
	// 搜索路径
	searchPaths := []string{
		".",                    // 当前目录
		"./config",             // config 子目录
		filepath.Join(os.Getenv("HOME"), ".totp_route"), // 用户主目录
		"/etc/totp_route",      // 系统配置目录
	}

	for _, path := range searchPaths {
		fullPath := filepath.Join(path, filename)
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath, nil
		}
	}

	return "", fmt.Errorf("在搜索路径中未找到配置文件 %s", filename)
}