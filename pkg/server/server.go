package server

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

// Server 服务端结构
type Server struct {
	config    *config.Config
	totp      *totp.Generator
	listeners map[int]*listener // offset -> listener
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// listener 监听器信息
type listener struct {
	listener   net.Listener
	port       int
	offset     int
	validStart int64
	validEnd   int64
}

// New 创建新的服务端实例
func New(cfg *config.Config) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		config:    cfg,
		totp:      totp.New(cfg.Secret, cfg.Interval),
		listeners: make(map[int]*listener),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start 启动服务端
func (s *Server) Start() error {
	log.Printf("服务端中间件启动（%s），动态监听 TOTP 端口并转发数据到目标 %s:%d",
		s.config.Protocol, s.config.Host, s.config.Port)

	if s.config.Protocol == "tcp" {
		return s.startTCP()
	} else if s.config.Protocol == "udp" {
		return s.startUDP()
	}

	return fmt.Errorf("不支持的协议: %s", s.config.Protocol)
}

// Stop 停止服务端
func (s *Server) Stop() {
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()

	// 关闭所有监听器
	for _, l := range s.listeners {
		if l.listener != nil {
			l.listener.Close()
		}
	}
	s.listeners = make(map[int]*listener)
}

// startTCP 启动TCP服务
func (s *Server) startTCP() error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return nil
		case <-ticker.C:
			s.updateTCPListeners()
		}
	}
}

// updateTCPListeners 更新TCP监听器
func (s *Server) updateTCPListeners() {
	now := time.Now().Unix()
	
	// 获取当前有效的端口信息
	validPorts, err := s.totp.GetValidPorts(s.config.Offsets, s.config.Extend, s.config.BasePort, s.config.PortRange)
	if err != nil {
		log.Printf("获取有效端口失败: %v", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 关闭过期的监听器
	for offset, l := range s.listeners {
		if now >= l.validEnd {
			l.listener.Close()
			delete(s.listeners, offset)
			log.Printf("[%s] 关闭过期监听器（偏移 %d，端口 %d）",
				time.Now().Format("15:04:05"), offset, l.port)
		}
	}

	// 创建新的监听器
	for _, portInfo := range validPorts {
		// 检查是否已经存在
		if _, exists := s.listeners[portInfo.Offset]; exists {
			continue
		}

		// 检查是否在有效期内
		if now < portInfo.ValidStart {
			continue
		}

		// 创建监听器
		tcpListener, err := net.Listen("tcp", fmt.Sprintf(":%d", portInfo.Port))
		if err != nil {
			log.Printf("创建TCP监听器失败（端口 %d）: %v", portInfo.Port, err)
			continue
		}

		l := &listener{
			listener:   tcpListener,
			port:       portInfo.Port,
			offset:     portInfo.Offset,
			validStart: portInfo.ValidStart,
			validEnd:   portInfo.ValidEnd,
		}

		s.listeners[portInfo.Offset] = l

		log.Printf("[%s] 创建TCP监听器（偏移 %d，端口 %d），有效期至 %s",
			time.Now().Format("15:04:05"), portInfo.Offset, portInfo.Port,
			time.Unix(portInfo.ValidEnd, 0).Format("15:04:05"))

		// 启动监听协程
		go s.handleTCPListener(l)
	}
}

// handleTCPListener 处理TCP监听器
func (s *Server) handleTCPListener(l *listener) {
	defer l.listener.Close()

	for {
		conn, err := l.listener.Accept()
		if err != nil {
			// 检查是否是因为监听器被关闭
			if opErr, ok := err.(*net.OpError); ok && opErr.Err.Error() == "use of closed network connection" {
				return
			}
			log.Printf("接受连接失败: %v", err)
			continue
		}

		log.Printf("[%s] 收到来自 %s 的连接（偏移 %d，端口 %d）",
			time.Now().Format("15:04:05"), conn.RemoteAddr(), l.offset, l.port)

		// 处理连接
		go s.handleTCPConnection(conn, l.offset, l.port)
	}
}

// handleTCPConnection 处理TCP连接
func (s *Server) handleTCPConnection(clientConn net.Conn, offset, listeningPort int) {
	defer clientConn.Close()

	// 连接到目标服务器
	targetConn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", s.config.Host, s.config.Port))
	if err != nil {
		log.Printf("连接目标服务器失败 %s:%d: %v", s.config.Host, s.config.Port, err)
		return
	}
	defer targetConn.Close()

	// 设置TCP优化选项
	s.setTCPOptions(clientConn)
	s.setTCPOptions(targetConn)

	// 双向转发
	s.bidirectionalCopy(clientConn, targetConn)

	log.Printf("[%s] 连接关闭，转发结束", time.Now().Format("15:04:05"))
}

// setTCPOptions 设置TCP优化选项
func (s *Server) setTCPOptions(conn net.Conn) {
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetNoDelay(true)
		tcpConn.SetReadBuffer(65536)
		tcpConn.SetWriteBuffer(65536)
	}
}

// bidirectionalCopy 双向数据复制
func (s *Server) bidirectionalCopy(conn1, conn2 net.Conn) {
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

// startUDP 启动UDP服务
func (s *Server) startUDP() error {
	udpListeners := make(map[int]*net.UDPConn)
	defer func() {
		for _, conn := range udpListeners {
			conn.Close()
		}
	}()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return nil
		case <-ticker.C:
			s.updateUDPListeners(udpListeners)
		}
	}
}

// updateUDPListeners 更新UDP监听器
func (s *Server) updateUDPListeners(udpListeners map[int]*net.UDPConn) {
	now := time.Now().Unix()
	
	// 获取当前有效的端口信息
	validPorts, err := s.totp.GetValidPorts(s.config.Offsets, s.config.Extend, s.config.BasePort, s.config.PortRange)
	if err != nil {
		log.Printf("获取有效端口失败: %v", err)
		return
	}

	// 关闭过期的UDP连接
	for offset, conn := range udpListeners {
		// 检查是否过期（简化处理，实际应该记录有效期）
		shouldClose := true
		for _, portInfo := range validPorts {
			if portInfo.Offset == offset && now < portInfo.ValidEnd {
				shouldClose = false
				break
			}
		}
		
		if shouldClose {
			conn.Close()
			delete(udpListeners, offset)
			log.Printf("[%s] 关闭过期UDP监听器（偏移 %d）",
				time.Now().Format("15:04:05"), offset)
		}
	}

	// 创建新的UDP监听器
	for _, portInfo := range validPorts {
		// 检查是否已经存在
		if _, exists := udpListeners[portInfo.Offset]; exists {
			continue
		}

		// 检查是否在有效期内
		if now < portInfo.ValidStart {
			continue
		}

		// 创建UDP监听器
		addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", portInfo.Port))
		if err != nil {
			log.Printf("解析UDP地址失败: %v", err)
			continue
		}

		conn, err := net.ListenUDP("udp", addr)
		if err != nil {
			log.Printf("创建UDP监听器失败（端口 %d）: %v", portInfo.Port, err)
			continue
		}

		udpListeners[portInfo.Offset] = conn

		log.Printf("[%s] 创建UDP监听器（偏移 %d，端口 %d），有效期至 %s",
			time.Now().Format("15:04:05"), portInfo.Offset, portInfo.Port,
			time.Unix(portInfo.ValidEnd, 0).Format("15:04:05"))

		// 启动处理协程
		go s.handleUDPListener(conn, portInfo.Offset, portInfo.Port)
	}
}

// handleUDPListener 处理UDP监听器
func (s *Server) handleUDPListener(conn *net.UDPConn, offset, port int) {
	buffer := make([]byte, 4096)
	
	for {
		n, clientAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			// 检查是否是因为连接被关闭
			if opErr, ok := err.(*net.OpError); ok && opErr.Err.Error() == "use of closed network connection" {
				return
			}
			log.Printf("UDP读取失败: %v", err)
			continue
		}

		if n > 0 {
			log.Printf("[%s] 收到来自 %s 的 UDP 数据包（偏移 %d，端口 %d）",
				time.Now().Format("15:04:05"), clientAddr, offset, port)

			// 处理UDP数据包
			go s.handleUDPPacket(conn, buffer[:n], clientAddr, offset, port)
		}
	}
}

// handleUDPPacket 处理UDP数据包
func (s *Server) handleUDPPacket(listener *net.UDPConn, data []byte, clientAddr *net.UDPAddr, offset, listeningPort int) {
	// 连接到目标服务器
	targetAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", s.config.Host, s.config.Port))
	if err != nil {
		log.Printf("解析目标UDP地址失败: %v", err)
		return
	}

	targetConn, err := net.DialUDP("udp", nil, targetAddr)
	if err != nil {
		log.Printf("连接目标UDP服务器失败: %v", err)
		return
	}
	defer targetConn.Close()

	// 发送数据到目标服务器
	_, err = targetConn.Write(data)
	if err != nil {
		log.Printf("发送UDP数据到目标服务器失败: %v", err)
		return
	}

	// 设置读取超时
	targetConn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// 读取响应
	response := make([]byte, 4096)
	n, err := targetConn.Read(response)
	if err != nil {
		log.Printf("读取目标服务器响应失败: %v", err)
		return
	}

	// 将响应发送回客户端
	_, err = listener.WriteToUDP(response[:n], clientAddr)
	if err != nil {
		log.Printf("发送UDP响应到客户端失败: %v", err)
		return
	}

	log.Printf("[%s] UDP 转发成功", time.Now().Format("15:04:05"))
}