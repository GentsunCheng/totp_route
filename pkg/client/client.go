package client

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"totp_route/pkg/config"
	"totp_route/pkg/totp"
)

// Client 客户端结构
type Client struct {
	config   *config.Config
	totp     *totp.Generator
	listener net.Listener
	ctx      context.Context
	cancel   context.CancelFunc
}

// New 创建新的客户端实例
func New(cfg *config.Config) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		config: cfg,
		totp:   totp.New(cfg.Secret, cfg.Interval),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start 启动客户端
func (c *Client) Start() error {
	log.Printf("客户端中间件启动（%s），监听本地端口 %d，转发数据至服务器 %s",
		c.config.Protocol, c.config.Port, c.config.Host)

	if c.config.Protocol == "tcp" {
		return c.startTCP()
	} else if c.config.Protocol == "udp" {
		return c.startUDP()
	}

	return fmt.Errorf("不支持的协议: %s", c.config.Protocol)
}

// Stop 停止客户端
func (c *Client) Stop() {
	c.cancel()
	if c.listener != nil {
		c.listener.Close()
	}
}

// startTCP 启动TCP客户端
func (c *Client) startTCP() error {
	// 创建本地监听器
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", c.config.Port))
	if err != nil {
		return fmt.Errorf("创建TCP监听器失败: %v", err)
	}
	c.listener = listener
	defer listener.Close()

	log.Printf("TCP客户端启动，监听端口 %d", c.config.Port)

	for {
		select {
		case <-c.ctx.Done():
			return nil
		default:
			// 设置接受连接的超时
			if tcpListener, ok := listener.(*net.TCPListener); ok {
				tcpListener.SetDeadline(time.Now().Add(1 * time.Second))
			}

			conn, err := listener.Accept()
			if err != nil {
				// 检查是否是超时错误
				if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
					continue
				}
				// 检查是否是因为监听器被关闭
				if opErr, ok := err.(*net.OpError); ok && opErr.Err.Error() == "use of closed network connection" {
					return nil
				}
				log.Printf("接受连接失败: %v", err)
				continue
			}

			log.Printf("[%s] 本地连接来自 %s", time.Now().Format("15:04:05"), conn.RemoteAddr())

			// 处理连接
			go c.handleTCPConnection(conn)
		}
	}
}

// handleTCPConnection 处理TCP连接
func (c *Client) handleTCPConnection(localConn net.Conn) {
	defer localConn.Close()

	// 尝试连接到服务器的TOTP动态端口
	serverConn, err := c.connectToServer()
	if err != nil {
		log.Printf("无法连接到服务器: %v", err)
		return
	}
	defer serverConn.Close()

	// 设置TCP优化选项
	c.setTCPOptions(localConn)
	c.setTCPOptions(serverConn)

	// 双向转发
	c.bidirectionalCopy(localConn, serverConn)

	log.Printf("[%s] 本地连接关闭，转发结束", time.Now().Format("15:04:05"))
}

// connectToServer 连接到服务器
func (c *Client) connectToServer() (net.Conn, error) {
	// 尝试所有偏移量
	for _, offset := range c.config.Offsets {
		port, err := c.totp.GetPortWithOffset(offset, c.config.BasePort, c.config.PortRange)
		if err != nil {
			log.Printf("计算TOTP端口失败（偏移 %d）: %v", offset, err)
			continue
		}

		serverAddr := fmt.Sprintf("%s:%d", c.config.Host, port)
		log.Printf("[%s] 尝试连接服务器 %s（偏移 %d）",
			time.Now().Format("15:04:05"), serverAddr, offset)

		conn, err := net.DialTimeout("tcp", serverAddr, 5*time.Second)
		if err != nil {
			log.Printf("连接 %s 失败: %v", serverAddr, err)
			continue
		}

		log.Printf("[%s] 连接成功：%s", time.Now().Format("15:04:05"), serverAddr)
		return conn, nil
	}

	return nil, fmt.Errorf("无法连接到服务器的任何TOTP端口")
}

// setTCPOptions 设置TCP优化选项
func (c *Client) setTCPOptions(conn net.Conn) {
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetNoDelay(true)
		tcpConn.SetReadBuffer(65536)
		tcpConn.SetWriteBuffer(65536)
	}
}

// bidirectionalCopy 双向数据复制
func (c *Client) bidirectionalCopy(conn1, conn2 net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	// conn1 -> conn2
	go func() {
		defer wg.Done()
		io.Copy(conn2, conn1)
		conn2.Close()
	}()

	// conn2 -> conn1
	go func() {
		defer wg.Done()
		io.Copy(conn1, conn2)
		conn1.Close()
	}()

	wg.Wait()
}

// startUDP 启动UDP客户端
func (c *Client) startUDP() error {
	// 创建本地UDP监听器
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", c.config.Port))
	if err != nil {
		return fmt.Errorf("解析UDP地址失败: %v", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("创建UDP监听器失败: %v", err)
	}
	defer conn.Close()

	log.Printf("UDP客户端启动，监听端口 %d", c.config.Port)

	buffer := make([]byte, 4096)
	for {
		select {
		case <-c.ctx.Done():
			return nil
		default:
			// 设置读取超时
			conn.SetReadDeadline(time.Now().Add(1 * time.Second))

			n, clientAddr, err := conn.ReadFromUDP(buffer)
			if err != nil {
				// 检查是否是超时错误
				if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
					continue
				}
				// 检查是否是因为连接被关闭
				if opErr, ok := err.(*net.OpError); ok && opErr.Err.Error() == "use of closed network connection" {
					return nil
				}
				log.Printf("UDP读取失败: %v", err)
				continue
			}

			if n > 0 {
				log.Printf("[%s] 本地UDP数据包来自 %s",
					time.Now().Format("15:04:05"), clientAddr)

				// 处理UDP数据包
				go c.handleUDPPacket(conn, buffer[:n], clientAddr)
			}
		}
	}
}

// handleUDPPacket 处理UDP数据包
func (c *Client) handleUDPPacket(localConn *net.UDPConn, data []byte, clientAddr *net.UDPAddr) {
	// 尝试连接到服务器的TOTP动态端口
	response, err := c.forwardUDPToServer(data)
	if err != nil {
		log.Printf("UDP转发到服务器失败: %v", err)
		return
	}

	// 将响应发送回客户端
	_, err = localConn.WriteToUDP(response, clientAddr)
	if err != nil {
		log.Printf("发送UDP响应到客户端失败: %v", err)
		return
	}

	log.Printf("[%s] UDP转发成功", time.Now().Format("15:04:05"))
}

// forwardUDPToServer 将UDP数据转发到服务器
func (c *Client) forwardUDPToServer(data []byte) ([]byte, error) {
	// 尝试所有偏移量
	for _, offset := range c.config.Offsets {
		port, err := c.totp.GetPortWithOffset(offset, c.config.BasePort, c.config.PortRange)
		if err != nil {
			log.Printf("计算TOTP端口失败（偏移 %d）: %v", offset, err)
			continue
		}

		serverAddr := fmt.Sprintf("%s:%d", c.config.Host, port)
		log.Printf("[%s] 尝试通过UDP连接服务器 %s（偏移 %d）",
			time.Now().Format("15:04:05"), serverAddr, offset)

		// 创建UDP连接
		udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
		if err != nil {
			log.Printf("解析服务器UDP地址失败: %v", err)
			continue
		}

		conn, err := net.DialUDP("udp", nil, udpAddr)
		if err != nil {
			log.Printf("连接服务器UDP失败: %v", err)
			continue
		}
		defer conn.Close()

		// 发送数据
		_, err = conn.Write(data)
		if err != nil {
			log.Printf("发送UDP数据到服务器失败: %v", err)
			conn.Close()
			continue
		}

		// 设置读取超时
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))

		// 读取响应
		response := make([]byte, 4096)
		n, err := conn.Read(response)
		if err != nil {
			log.Printf("读取服务器UDP响应失败: %v", err)
			conn.Close()
			continue
		}

		conn.Close()
		log.Printf("[%s] UDP转发成功，使用偏移 %d", time.Now().Format("15:04:05"), offset)
		return response[:n], nil
	}

	return nil, fmt.Errorf("无法通过UDP连接到服务器的任何TOTP端口")
}

// GetServerPorts 获取当前服务器端口列表（用于调试）
func (c *Client) GetServerPorts() ([]int, error) {
	var ports []int
	for _, offset := range c.config.Offsets {
		port, err := c.totp.GetPortWithOffset(offset, c.config.BasePort, c.config.PortRange)
		if err != nil {
			continue
		}
		ports = append(ports, port)
	}
	return ports, nil
}

// TestServerConnection 测试服务器连接（用于调试）
func (c *Client) TestServerConnection() error {
	for _, offset := range c.config.Offsets {
		port, err := c.totp.GetPortWithOffset(offset, c.config.BasePort, c.config.PortRange)
		if err != nil {
			log.Printf("计算TOTP端口失败（偏移 %d）: %v", offset, err)
			continue
		}

		serverAddr := fmt.Sprintf("%s:%d", c.config.Host, port)
		log.Printf("测试连接到 %s（偏移 %d）", serverAddr, offset)

		if c.config.Protocol == "tcp" {
			conn, err := net.DialTimeout("tcp", serverAddr, 3*time.Second)
			if err != nil {
				log.Printf("TCP连接失败: %v", err)
				continue
			}
			conn.Close()
			log.Printf("TCP连接成功")
			return nil
		} else if c.config.Protocol == "udp" {
			udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
			if err != nil {
				log.Printf("解析UDP地址失败: %v", err)
				continue
			}

			conn, err := net.DialUDP("udp", nil, udpAddr)
			if err != nil {
				log.Printf("UDP连接失败: %v", err)
				continue
			}

			// 发送测试数据
			testData := []byte("test")
			_, err = conn.Write(testData)
			if err != nil {
				log.Printf("发送UDP测试数据失败: %v", err)
				conn.Close()
				continue
			}

			// 设置短暂的读取超时
			conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			buffer := make([]byte, 1024)
			_, err = conn.Read(buffer)
			conn.Close()

			if err != nil {
				log.Printf("UDP测试可能成功（无响应是正常的）")
			} else {
				log.Printf("UDP连接成功")
			}
			return nil
		}
	}

	return fmt.Errorf("无法连接到服务器的任何TOTP端口")
}