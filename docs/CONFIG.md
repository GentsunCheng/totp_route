# 配置文档

## 配置文件说明

TOTP Route 使用 TOML 格式的配置文件。程序首次运行时会自动从 `config.toml.example` 复制创建 `config.toml`。

## 配置参数详解

### 基本参数

#### `interval` (整数，默认: 30)
TOTP 算法的时间间隔，单位为秒。
- **作用**: 决定动态端口的变化频率
- **建议值**: 30-60秒，太短会频繁切换端口，太长安全性降低
- **注意**: 服务端和客户端必须设置相同的值

#### `extend` (整数，默认: 15)
时间窗口的扩展时间，单位为秒。
- **作用**: 在时间窗口前后增加宽容时间，防止时间不同步导致连接失败
- **建议值**: interval的一半，即15秒（当interval为30时）
- **注意**: 过大会降低安全性，过小可能导致连接不稳定

#### `base_port` (整数，默认: 3000)
动态端口计算的基础端口号。
- **作用**: 所有动态端口都会在此基础上计算
- **范围**: 1-65535，但建议使用1024以上的端口
- **注意**: 确保端口范围不与其他服务冲突

#### `port_range` (整数，默认: 1000)
动态端口的范围大小。
- **作用**: 动态端口会在 [base_port, base_port + port_range) 范围内计算
- **安全性**: 范围越大，端口预测难度越高
- **注意**: base_port + port_range 不能超过65535

#### `secret` (字符串，必填)
TOTP 算法使用的密钥。
- **重要性**: ⚠️ 服务端和客户端必须使用相同的密钥
- **安全性**: 应使用强随机字符串，建议长度16-32字符
- **生成方式**: 可使用在线TOTP密钥生成器或自定义字符串
- **保密**: 严格保密，不要在日志或代码中暴露

### 网络参数

#### `host` (字符串，默认: "127.0.0.1")
主机地址，含义取决于运行模式：
- **服务端模式**: 目标服务的地址（要转发到的服务）
- **客户端模式**: 服务端的地址（TOTP Route服务端）

#### `port` (整数，默认: 8080)
端口号，含义取决于运行模式：
- **服务端模式**: 目标服务的端口（要转发到的服务端口）
- **客户端模式**: 本地监听端口（客户端应用连接的端口）

#### `protocol` (字符串，默认: "tcp")
网络传输协议。
- **可选值**: "tcp" 或 "udp"
- **TCP**: 可靠连接，适合大多数应用（SSH、HTTP等）
- **UDP**: 无连接，适合实时应用（游戏、语音等）

### 运行参数

#### `mode` (字符串，默认: "server")
程序运行模式。
- **server**: 服务端模式，监听动态端口并转发到目标服务
- **client**: 客户端模式，监听固定端口并转发到服务端动态端口

#### `offsets` (整数数组，默认: [-15, 0, 15])
时间偏移量数组，单位为秒。
- **作用**: 程序会同时监听/尝试连接多个时间偏移对应的端口
- **容错**: 提高连接成功率，应对时间不同步问题
- **建议**: 使用[-15, 0, 15]可应对±15秒的时间误差

## 配置示例

### SSH 隧道配置

#### 服务端配置 (server.toml)
```toml
# SSH 服务保护配置
interval = 30
extend = 15
base_port = 3000
port_range = 1000
secret = 'MySSHSecret123456'
offsets = [-15, 0, 15]

# 目标SSH服务
host = '127.0.0.1'
port = 22

# 运行模式
mode = 'server'
protocol = 'tcp'
```

#### 客户端配置 (client.toml)
```toml
# SSH 客户端配置
interval = 30
extend = 15
base_port = 3000
port_range = 1000
secret = 'MySSHSecret123456'  # 与服务端相同
offsets = [-15, 0, 15]

# 服务端地址
host = '192.168.1.100'  # 服务端IP
port = 2222             # 本地SSH端口

# 运行模式
mode = 'client'
protocol = 'tcp'
```

### Web 服务保护配置

#### 服务端配置
```toml
interval = 60           # Web服务可使用较长间隔
extend = 30
base_port = 4000
port_range = 2000
secret = 'WebServiceSecret789'
offsets = [-30, 0, 30]

# 目标Web服务
host = '127.0.0.1'
port = 80

mode = 'server'
protocol = 'tcp'
```

#### 客户端配置
```toml
interval = 60
extend = 30
base_port = 4000
port_range = 2000
secret = 'WebServiceSecret789'
offsets = [-30, 0, 30]

# 服务端地址
host = 'web-server.example.com'
port = 8080             # 本地Web代理端口

mode = 'client'
protocol = 'tcp'
```

### UDP 游戏服务配置

#### 服务端配置
```toml
interval = 20           # 游戏服务使用较短间隔
extend = 10
base_port = 5000
port_range = 1000
secret = 'GameServerSecret456'
offsets = [-10, 0, 10]

# 目标游戏服务
host = '127.0.0.1'
port = 25565           # Minecraft服务端口

mode = 'server'
protocol = 'udp'       # 使用UDP协议
```

#### 客户端配置
```toml
interval = 20
extend = 10
base_port = 5000
port_range = 1000
secret = 'GameServerSecret456'
offsets = [-10, 0, 10]

# 服务端地址
host = 'game-server.example.com'
port = 25565           # 本地游戏端口

mode = 'client'
protocol = 'udp'
```

## 安全最佳实践

### 1. 密钥管理
- 使用强随机密钥，至少16字符
- 定期更换密钥
- 不要在配置文件中使用默认密钥
- 通过安全渠道分发密钥

### 2. 端口配置
- 选择不常用的端口范围
- 避免与系统服务端口冲突
- 端口范围要足够大以增加安全性

### 3. 时间同步
- 确保服务端和客户端时间同步
- 使用NTP服务
- 监控时间偏差

### 4. 网络安全
- 在不安全网络中使用VPN
- 配置防火墙规则
- 限制访问IP范围

## 故障排除

### 连接失败
1. 检查密钥是否一致
2. 验证时间同步
3. 确认网络连通性
4. 检查防火墙设置

### 端口冲突
1. 修改base_port或port_range
2. 检查其他程序占用
3. 使用netstat查看端口状态

### 配置错误
1. 使用测试模式验证: `./totp_route -t`
2. 检查配置文件语法
3. 查看程序日志输出

## 高级配置

### 多实例部署
可以运行多个实例保护不同服务：
```bash
# SSH保护
./totp_route -c ssh.toml

# Web保护 
./totp_route -c web.toml

# 数据库保护
./totp_route -c db.toml
```

### 负载均衡
配置多个客户端连接同一服务端，实现简单的负载均衡。

### 监控和日志
- 程序会输出详细的连接日志
- 可以重定向到日志文件进行分析
- 建议配置日志轮转

```bash
# 日志记录示例
./totp_route -c config.toml >> totp_route.log 2>&1
```