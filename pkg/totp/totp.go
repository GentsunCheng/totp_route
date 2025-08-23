package totp

import (
	"strconv"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// Generator TOTP生成器
type Generator struct {
	secret   string
	interval int
}

// New 创建新的TOTP生成器
func New(secret string, interval int) *Generator {
	return &Generator{
		secret:   secret,
		interval: interval,
	}
}

// GenerateCode 生成指定时间的TOTP代码
func (g *Generator) GenerateCode(t time.Time) (string, error) {
	opts := totp.ValidateOpts{
		Period:    uint(g.interval),
		Skew:      0,
		Digits:    6,
		Algorithm: otp.AlgorithmSHA1,
	}
	
	return totp.GenerateCodeCustom(g.secret, t, opts)
}

// GetPortOffset 根据TOTP代码计算端口偏移量
func (g *Generator) GetPortOffset(t time.Time, portRange int) (int, error) {
	code, err := g.GenerateCode(t)
	if err != nil {
		return 0, err
	}
	
	// 将TOTP代码转换为整数
	codeInt, err := strconv.Atoi(code)
	if err != nil {
		return 0, err
	}
	
	// 计算端口偏移量
	return codeInt % portRange, nil
}

// GetPort 根据TOTP计算动态端口
func (g *Generator) GetPort(t time.Time, basePort, portRange int) (int, error) {
	offset, err := g.GetPortOffset(t, portRange)
	if err != nil {
		return 0, err
	}
	
	return basePort + offset, nil
}

// GetWindowParams 获取时间窗口参数
func (g *Generator) GetWindowParams(offset, extend int) (windowStart, validStart, validEnd int64) {
	now := time.Now().Unix()
	t := now + int64(offset)
	
	// 计算时间窗口开始时间（对齐到间隔）
	windowStart = (t / int64(g.interval)) * int64(g.interval)
	
	// 计算有效期
	validStart = windowStart - int64(extend)
	validEnd = windowStart + int64(g.interval) + int64(extend)
	
	return windowStart, validStart, validEnd
}

// GetPortForWindow 获取指定时间窗口的端口
func (g *Generator) GetPortForWindow(windowStart int64, basePort, portRange int) (int, error) {
	windowTime := time.Unix(windowStart, 0)
	return g.GetPort(windowTime, basePort, portRange)
}

// ValidateCode 验证TOTP代码
func (g *Generator) ValidateCode(code string, t time.Time) bool {
	opts := totp.ValidateOpts{
		Period:    uint(g.interval),
		Skew:      1, // 允许前后一个时间窗口的误差
		Digits:    6,
		Algorithm: otp.AlgorithmSHA1,
	}
	
	valid, err := totp.ValidateCustom(code, g.secret, t, opts)
	if err != nil {
		return false
	}
	
	return valid
}

// GetCurrentPort 获取当前时间的动态端口
func (g *Generator) GetCurrentPort(basePort, portRange int) (int, error) {
	return g.GetPort(time.Now(), basePort, portRange)
}

// GetPortWithOffset 获取带时间偏移的动态端口
func (g *Generator) GetPortWithOffset(offset, basePort, portRange int) (int, error) {
	targetTime := time.Now().Add(time.Duration(offset) * time.Second)
	return g.GetPort(targetTime, basePort, portRange)
}

// IsPortValid 检查端口在指定时间是否有效
func (g *Generator) IsPortValid(port, basePort, portRange int, t time.Time, extend int) bool {
	// 计算当前时间应该的端口
	expectedPort, err := g.GetPort(t, basePort, portRange)
	if err != nil {
		return false
	}
	
	if port == expectedPort {
		return true
	}
	
	// 检查扩展时间窗口内的端口
	for i := -extend; i <= extend; i++ {
		checkTime := t.Add(time.Duration(i) * time.Second)
		checkPort, err := g.GetPort(checkTime, basePort, portRange)
		if err != nil {
			continue
		}
		if port == checkPort {
			return true
		}
	}
	
	return false
}

// PortInfo 端口信息
type PortInfo struct {
	Port       int   // 端口号
	ValidStart int64 // 有效开始时间
	ValidEnd   int64 // 有效结束时间
	Offset     int   // 时间偏移量
}

// GetValidPorts 获取所有有效的端口信息
func (g *Generator) GetValidPorts(offsets []int, extend, basePort, portRange int) ([]PortInfo, error) {
	var ports []PortInfo
	
	for _, offset := range offsets {
		windowStart, validStart, validEnd := g.GetWindowParams(offset, extend)
		port, err := g.GetPortForWindow(windowStart, basePort, portRange)
		if err != nil {
			continue
		}
		
		ports = append(ports, PortInfo{
			Port:       port,
			ValidStart: validStart,
			ValidEnd:   validEnd,
			Offset:     offset,
		})
	}
	
	return ports, nil
}