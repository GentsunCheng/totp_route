@echo off
REM totp_route 构建脚本 for Windows

echo =================================
echo  TOTP Route 构建脚本
echo =================================

echo.
echo 1. 检查Go环境...
go version
if %errorlevel% neq 0 (
    echo 错误: 未安装Go或Go不在PATH中
    pause
    exit /b 1
)

echo.
echo 2. 设置Go代理（解决网络问题）...
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GOSUMDB=sum.golang.google.cn

echo.
echo 3. 下载依赖...
go mod tidy
if %errorlevel% neq 0 (
    echo 错误: 依赖下载失败，请检查网络连接
    echo 如果问题持续，请尝试以下命令：
    echo   go env -w GOPROXY=direct
    pause
    exit /b 1
)

echo.
echo 4. 编译程序...
go build -o totp_route.exe ./cmd
if %errorlevel% neq 0 (
    echo 错误: 编译失败
    pause
    exit /b 1
)

echo.
echo ✓ 构建成功！
echo   可执行文件: totp_route.exe
echo   使用方法: totp_route.exe -h

echo.
echo 5. 测试程序...
totp_route.exe -v

echo.
echo =================================
echo  构建完成
echo =================================
pause