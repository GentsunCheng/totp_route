# totp_route Makefile

# 变量定义
BINARY_NAME=totp_route
MAIN_PATH=./cmd
BUILD_DIR=./build
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "v1.0.0")
LDFLAGS=-ldflags "-X main.version=$(VERSION)"

# 检测操作系统并设置可执行文件后缀
# Windows下使用 make build-windows️
# Linux/macOS下使用 make build-unix

# 默认目标
.PHONY: all
all: clean deps build-auto

# 自动检测平台构建
.PHONY: build-auto
build-auto:
	@echo "自动检测平台并构建..."
	@powershell -Command "if ($$env:OS -eq 'Windows_NT') { make build-win } else { make build-unix }" 2>/dev/null || make build-unix

# 清理构建文件
.PHONY: clean
clean:
	@echo "清理构建文件..."
	@rm -rf $(BUILD_DIR) 2>/dev/null || true
	@rm -f $(BINARY_NAME) 2>/dev/null || true
	@rm -f $(BINARY_NAME).exe 2>/dev/null || true

# 下载依赖
.PHONY: deps
deps:
	@echo "下载依赖..."
	@go mod tidy

# Windows构建
.PHONY: build-win
build-win:
	@echo "构建 Windows 版本..."
	@go build $(LDFLAGS) -o $(BINARY_NAME).exe $(MAIN_PATH)

# Unix构建 (Linux/macOS)
.PHONY: build-unix
build-unix:
	@echo "构建 Unix 版本..."
	@go build $(LDFLAGS) -o $(BINARY_NAME) $(MAIN_PATH)

# 用于向后兼容的构建目标
.PHONY: build
build: build-auto

# Windows构建
.PHONY: build-windows
build-windows:
	@echo "构建 Windows 版本..."
	@GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)

# Linux构建
.PHONY: build-linux
build-linux:
	@echo "构建 Linux 版本..."
	@GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)

# Mac构建
.PHONY: build-darwin
build-darwin:
	@echo "构建 macOS 版本..."
	@GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)

# 交叉编译所有平台
.PHONY: build-all
build-all: clean deps
	@mkdir -p $(BUILD_DIR)
	@$(MAKE) build-windows
	@$(MAKE) build-linux
	@$(MAKE) build-darwin
	@echo "所有平台构建完成！"

# 运行测试
.PHONY: test
test:
	@echo "运行测试..."
	@go test -v ./...

# 代码格式化
.PHONY: fmt
fmt:
	@echo "格式化代码..."
	@go fmt ./...

# 代码检查
.PHONY: vet
vet:
	@echo "代码检查..."
	@go vet ./...

# 运行程序（测试模式）
.PHONY: run-test
run-test: build
	@echo "运行测试模式..."
	@if [ -f "$(BINARY_NAME).exe" ]; then ./$(BINARY_NAME).exe -t; else ./$(BINARY_NAME) -t; fi

# 显示帮助
.PHONY: help
help:
	@echo "可用命令："
	@echo "  make all          - 清理、下载依赖并构建"
	@echo "  make build        - 构建当前平台版本"
	@echo "  make build-all    - 交叉编译所有平台版本"
	@echo "  make build-windows- 构建Windows版本"
	@echo "  make build-linux  - 构建Linux版本"
	@echo "  make build-darwin - 构建macOS版本"
	@echo "  make test         - 运行测试"
	@echo "  make fmt          - 格式化代码"
	@echo "  make vet          - 代码检查"
	@echo "  make clean        - 清理构建文件"
	@echo "  make deps         - 下载依赖"
	@echo "  make run-test     - 运行程序测试模式"
	@echo "  make help         - 显示此帮助信息"

# 安装到系统和卸载（仅支持Unix系统）
.PHONY: install
install: build-unix
	@echo "安装到系统..."
	@sudo cp $(BINARY_NAME) /usr/local/bin/

.PHONY: uninstall
uninstall:
	@echo "从系统卸载..."
	@sudo rm -f /usr/local/bin/$(BINARY_NAME)