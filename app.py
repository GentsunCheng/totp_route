import sys
import toml
import shutil
import argparse

def load_config(config_file):
    """
    从 toml 文件中加载配置参数
    配置项示例：
        interval = 30
        extend = 15
        base_port = 3000
        port_range = 1000
        secret = 'S3K3TPI5MYA2M67V'
        offsets = [-15, 0, 15]
        host = '127.0.0.1'       # 服务端模式下为目标转发地址，客户端模式下为服务端 IP
        port = 8080              # 服务端模式下为目标转发端口，客户端模式下为本地监听端口
        mode = 'server'          # server 或 client
    """
    try:
        config = toml.load(config_file)
        return config
    except Exception as e:
        print(f"加载配置文件 {config_file} 失败: {e}")
        sys.exit(1)

if __name__ == '__main__':
    # 默认配置文件名为 config.toml，你也可以通过命令行参数指定
    parser = argparse.ArgumentParser(description='Load config file')
    parser.add_argument('-c', '--config', type=str, default='config.toml', help='Path to config file')
    args = parser.parse_args()

    if not args.config:
        config_file = 'config.toml'
    else:
        config_file = args.config
    try:
        config = load_config(args.config)
    except Exception as e:
        if config_file == 'config.toml':
            try:
                shutil.copy('config.toml.example', 'config.toml')
                print("初始化程序配置文件 config.toml，请根据需要修改配置参数.")
                sys.exit(0)
            except Exception as e:
                print(f"配置文件 config.toml.example 不存在，请重新下载程序: {e}")
                sys.exit(1)
        else:
            print(f"加载配置文件 {config_file} 失败: {e}")
            sys.exit(1)
    

    # 从配置中读取参数
    interval   = config.get('interval', 30)
    extend     = config.get('extend', 15)
    base_port  = config.get('base_port', 3000)
    port_range = config.get('port_range', 1000)
    secret     = config.get('secret', '')
    offsets    = config.get('offsets', [-15, 0, 15])
    host       = config.get('host', '127.0.0.1')
    port       = config.get('port', 8080)
    mode       = config.get('mode', 'server').lower()

    if mode == 'server':
        # 在服务端模式下，host 与 port 分别为目标转发地址与目标转发端口
        from modules.route.route import Server
        app = Server(interval=interval, extend=extend, base_port=base_port, port_range=port_range,
                        secret=secret, offsets=offsets, target_host=host, target_port=port)
    elif mode == 'client':
        # 在客户端模式下，host 为服务端 IP，port 为本地监听端口
        from modules.route.route import Client
        app = Client(interval=interval, extend=extend, base_port=base_port, port_range=port_range,
                        secret=secret, offsets=offsets, server_ip=host, local_listen_port=port)
    else:
        print("无效的 mode 配置，请设置为 'server' 或 'client'")
        sys.exit(1)

    try:
        app.run()
    except KeyboardInterrupt:
        print("程序已终止")
        sys.exit(0)