# 以迁移至[OkaRoute](https://github.com/GentsunCheng/OkaRoute)

# TOTP Route

基于TOTP（Time-based One-Time Password）的随机端口流量转发工具

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/go-%3E%3D1.21-blue.svg)
![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20Linux%20%7C%20macOS-lightgrey.svg)

## 🎯 项目简介

TOTP Route 是一个基于时间基础一次性密码（TOTP）算法的网络流量转发工具。通过动态端口机制，显著提升网络通信的安全性，防止固定端口被扫描和攻击。

### ✨ 主要特性

- 🔐 **动态端口**: 基于TOTP算法动态计算监听端口
- 🚀 **双模式支持**: 服务端和客户端模式
- 🌐 **协议支持**: TCP和UDP协议
- ⚡ **高性能**: 优化的并发处理和网络传输
- 🔧 **易配置**: 简单的TOML配置文件
- 🖥️ **跨平台**: 支持Windows、Linux、macOS
- 📊 **详细日志**: 完整的连接和转发日志

### 🏗️ 架构说明

```
客户端应用 <---> TOTP Route Client <---> 网络 <---> TOTP Route Server <---> 目标服务
             (本地固定端口)                        (动态TOTP端口)         (目标端口)
```

## 🚀 快速开始

### 环境要求

- Go 1.21 或更高版本
- 网络连接（用于下载依赖）

### 📦 安装方式

#### 方式一：源码编译

```bash
# 克隆仓库
git clone https://github.com/your-repo/totp_route.git
cd totp_route

# 使用构建脚本（推荐）
# Windows
.\build.bat

# Linux/macOS
./build.sh

# 或使用Makefile
make all
```

#### 方式二：直接构建

```bash
# 下载依赖
go mod tidy

# 编译
# Windows
go build -o totp_route.exe ./cmd

# Linux/macOS
go build -o totp_route ./cmd
```

### ⚙️ 配置文件

程序首次运行时会自动创建配置文件 `config.toml`：

```toml
interval = 30              # TOTP时间间隔（秒）
extend = 15                # 时间窗口扩展（秒）
base_port = 3000           # 基础端口
port_range = 1000          # 端口范围
secret = 'YOUR_SECRET_KEY' # TOTP密钥（服务端和客户端必须相同）
offsets = [-15, 0, 15]     # 时间偏移量（秒）
host = '127.0.0.1'         # 主机地址
port = 8080                # 端口
mode = 'server'            # 运行模式：server 或 client
protocol = 'tcp'           # 协议：tcp 或 udp
```

#### 配置说明

| 参数 | 说明 | 服务端 | 客户端 |
|------|------|--------|--------|
| `host` | 主机地址 | 目标服务地址 | 服务端地址 |
| `port` | 端口 | 目标服务端口 | 本地监听端口 |
| `secret` | TOTP密钥 | ⚠️ 必须与客户端相同 | ⚠️ 必须与服务端相同 |

### 🔧 使用方法

#### 1. 服务端模式

编辑 `config.toml`：
```toml
mode = 'server'
host = '192.168.1.100'  # 目标服务地址
port = 22               # 目标服务端口（如SSH）
secret = 'SHARED_SECRET_KEY'
```

运行服务端：
```bash
# Windows
.\totp_route.exe -c config.toml

# Linux/macOS
./totp_route -c config.toml
```

#### 2. 客户端模式

编辑 `config.toml`：
```toml
mode = 'client'
host = '服务端IP地址'     # 服务端地址
port = 2222             # 本地监听端口
secret = 'SHARED_SECRET_KEY'  # 与服务端相同的密钥
```

运行客户端：
```bash
# Windows
.\totp_route.exe -c config.toml

# Linux/macOS
./totp_route -c config.toml
```

#### 3. 连接目标服务

客户端运行后，连接到客户端的本地端口：
```bash
# 例如：SSH通过TOTP Route连接
ssh user@localhost -p 2222
```

### 📋 命令行选项

```bash
./totp_route [选项]

选项：
  -c <文件>    指定配置文件路径 (默认: config.toml)
  -t           测试模式，验证配置和连接
  -v           显示版本信息
  -h           显示帮助信息
```

### 🧪 测试配置

运行测试模式验证配置：
```bash
# Windows
.\totp_route.exe -t -c config.toml

# Linux/macOS
./totp_route -t -c config.toml
```

## 📚 使用示例

### 示例1：保护SSH服务

**服务端配置** (`server.toml`)：
```toml
mode = 'server'
protocol = 'tcp'
host = '127.0.0.1'    # SSH服务地址
port = 22             # SSH端口
base_port = 3000
port_range = 1000
secret = 'MySecretKey123'
interval = 30
extend = 15
offsets = [-15, 0, 15]
```

**客户端配置** (`client.toml`)：
```toml
mode = 'client'
protocol = 'tcp'
host = '服务器IP'      # 服务端地址
port = 2222           # 本地监听端口
base_port = 3000
port_range = 1000
secret = 'MySecretKey123'  # 与服务端相同
interval = 30
extend = 15
offsets = [-15, 0, 15]
```

**使用方法**：
```bash
# 服务端
./totp_route -c server.toml

# 客户端
./totp_route -c client.toml

# 连接SSH
ssh user@localhost -p 2222
```

### 示例2：保护Web服务

**服务端配置**：
```toml
mode = 'server'
protocol = 'tcp'
host = '127.0.0.1'
port = 80             # Web服务端口
secret = 'WebSecret456'
# 其他参数...
```

**客户端配置**：
```toml
mode = 'client'
protocol = 'tcp'
host = '服务器IP'
port = 8080           # 本地代理端口
secret = 'WebSecret456'
# 其他参数...
```

**访问方式**：
```
http://localhost:8080  # 通过TOTP Route访问远程Web服务
```

## 🔨 开发构建

### 构建工具

项目提供多种构建方式：

```bash
# 使用Makefile（推荐）
make help          # 查看所有可用命令
make all           # 清理、下载依赖并构建
make build-all     # 交叉编译所有平台
make test          # 运行测试
make fmt           # 代码格式化

# 使用构建脚本
./build.sh         # Linux/macOS
.\build.bat        # Windows
```

### 项目结构

```
totp_route/
├── cmd/
│   └── main.go              # 主程序入口
├── pkg/
│   ├── config/
│   │   └── config.go        # 配置管理
│   ├── totp/
│   │   └── totp.go          # TOTP算法实现
│   ├── server/
│   │   └── server.go        # 服务端实现
│   └── client/
│       └── client.go        # 客户端实现
├── config.toml.example      # 配置文件示例
├── go.mod                   # Go模块文件
├── go.sum                   # 依赖校验文件
├── Makefile                 # 构建文件
├── build.sh                 # Linux/macOS构建脚本
├── build.bat                # Windows构建脚本
└── README.md                # 项目文档
```

## 🔒 安全考虑

1. **密钥安全**: TOTP密钥应当保密，服务端和客户端必须使用相同的密钥
2. **时间同步**: 确保服务端和客户端的系统时间同步
3. **网络安全**: 在不安全的网络环境中建议使用VPN等额外保护
4. **端口范围**: 选择合适的端口范围，避免与其他服务冲突

## 🐛 故障排除

### 常见问题

**Q: 客户端无法连接到服务端**
- 检查网络连通性
- 确认TOTP密钥一致
- 验证系统时间同步
- 检查防火墙设置

**Q: 编译时网络错误**
- 检查网络连接
- 设置Go代理：`go env -w GOPROXY=https://goproxy.cn,direct`
- 重新运行 `go mod tidy`

**Q: 端口被占用**
- 修改配置文件中的端口范围
- 检查其他程序是否占用端口

### 调试模式

使用测试模式进行故障诊断：
```bash
./totp_route -t -c config.toml
```

## 🤝 贡献指南

欢迎贡献代码！请遵循以下步骤：

1. Fork 项目
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 开启 Pull Request

## 📄 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情。

## 🙏 致谢

- [pquerna/otp](https://github.com/pquerna/otp) - TOTP算法实现
- [spf13/viper](https://github.com/spf13/viper) - 配置文件处理

## 📞 联系方式

- 项目主页: https://github.com/your-repo/totp_route
- 问题反馈: https://github.com/your-repo/totp_route/issues

---

**⚠️ 免责声明**: 本工具仅供学习和研究使用，请在法律允许的范围内使用。
