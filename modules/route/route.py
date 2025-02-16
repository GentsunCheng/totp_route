#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import selectors
import socket
import time
import threading
import pyotp
import toml
import sys

def bidirectional_forward(conn1, conn2):
    """
    双向转发函数，启动两个线程分别转发 conn1->conn2 与 conn2->conn1，
    直到任一端断开。
    """
    def forward(src, dst):
        try:
            while True:
                data = src.recv(4096)
                if not data:
                    break
                dst.sendall(data)
        except Exception:
            pass
        finally:
            try:
                dst.shutdown(socket.SHUT_WR)
            except Exception:
                pass

    t1 = threading.Thread(target=forward, args=(conn1, conn2))
    t2 = threading.Thread(target=forward, args=(conn2, conn1))
    t1.start()
    t2.start()
    t1.join()
    t2.join()

class Server:
    """
    Server 类：基于 TOTP 动态端口，在有效期内监听客户端连接，
    接收到连接后将数据转发至目标地址。
    """
    def __init__(self, interval, extend, base_port, port_range, secret, offsets, target_host, target_port):
        self.interval = interval
        self.extend = extend
        self.base_port = base_port
        self.port_range = port_range
        self.secret = secret
        self.offsets = offsets
        self.target_host = target_host
        self.target_port = target_port

        self.sockets = {}  # offset -> (sock, valid_start, valid_end, port)
        self.sel = selectors.DefaultSelector()

    def get_window_params(self, offset):
        """
        根据当前时间加上偏移，计算 TOTP 窗口的起始时间、有效期和映射端口。
        有效期为：窗口开始前 extend 秒 到窗口结束后 extend 秒
        """
        now = time.time()
        t = now + offset
        window_start = (int(t) // self.interval) * self.interval
        valid_start = window_start - self.extend
        valid_end   = window_start + self.interval + self.extend

        totp = pyotp.TOTP(self.secret, interval=self.interval)
        otp = totp.at(window_start)
        port_offset = int(otp) % self.port_range
        port = self.base_port + port_offset

        return port, valid_start, valid_end

    def create_socket(self, port):
        """
        创建并返回一个非阻塞的 TCP 监听 socket
        """
        s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        s.setblocking(False)
        s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        s.bind(('0.0.0.0', port))
        s.listen(5)
        return s

    def handle_connection(self, client_conn, offset, listening_port):
        """
        当有客户端连接到 TOTP 监听端口后，建立到目标地址的连接，并进行双向数据转发。
        """
        print(f"[{time.strftime('%H:%M:%S')}] 收到来自 {client_conn.getpeername()} 的连接（偏移 {offset}，监听端口 {listening_port}）")
        try:
            target_conn = socket.create_connection((self.target_host, self.target_port))
        except Exception as e:
            print(f"连接目标 {self.target_host}:{self.target_port} 失败: {e}")
            client_conn.close()
            return

        bidirectional_forward(client_conn, target_conn)
        client_conn.close()
        target_conn.close()
        print(f"[{time.strftime('%H:%M:%S')}] 连接关闭，转发结束。")

    def run(self):
        print("服务端中间件启动，动态监听 TOTP 端口并转发数据到目标地址...")
        while True:
            now = time.time()
            next_timeout = 5

            # 更新或创建各偏移对应的监听 socket
            for offset in self.offsets:
                port, valid_start, valid_end = self.get_window_params(offset)
                info = self.sockets.get(offset)
                if info:
                    sock, old_valid_start, old_valid_end, old_port = info
                    # 如果已超出有效期或端口变更（进入新窗口），则关闭旧 socket
                    if now >= old_valid_end or port != old_port:
                        try:
                            self.sel.unregister(sock)
                        except Exception:
                            pass
                        sock.close()
                        print(f"[{time.strftime('%H:%M:%S')}] 关闭偏移 {offset} 的旧 socket（端口 {old_port}）")
                        del self.sockets[offset]
                        info = None
                if not info:
                    # 如果已到监听时间，则创建新 socket
                    if now >= valid_start:
                        try:
                            s = self.create_socket(port)
                            self.sel.register(s, selectors.EVENT_READ, data=offset)
                            self.sockets[offset] = (s, valid_start, valid_end, port)
                            print(f"[{time.strftime('%H:%M:%S')}] 创建监听 socket，偏移 {offset}，端口 {port}，有效期至 {time.strftime('%H:%M:%S', time.localtime(valid_end))}")
                        except Exception as e:
                            print(f"创建端口 {port} 失败: {e}")
                    else:
                        next_timeout = min(next_timeout, valid_start - now)

            # 根据各 socket 的剩余有效期，更新下一次超时时间
            for offset, (s, valid_start, valid_end, port) in self.sockets.items():
                time_to_expiry = valid_end - now
                next_timeout = min(next_timeout, time_to_expiry)

            if not self.sockets:
                time.sleep(next_timeout)
                continue

            events = self.sel.select(timeout=next_timeout)
            for key, mask in events:
                s = key.fileobj
                offset = key.data
                try:
                    client_conn, addr = s.accept()
                    client_conn.setblocking(True)
                    t = threading.Thread(target=self.handle_connection, args=(client_conn, offset, s.getsockname()[1]))
                    t.start()
                except Exception as e:
                    print("接受连接时出错:", e)

class Client:
    """
    Client 类：监听本地端口，接收到本地连接后尝试连接服务端的 TOTP 动态端口，
    并进行数据转发。
    """
    def __init__(self, interval, extend, base_port, port_range, secret, offsets, server_ip, local_listen_port):
        self.interval = interval
        self.extend = extend  # 虽然客户端中未用到 extend，但为保持配置统一保留
        self.base_port = base_port
        self.port_range = port_range
        self.secret = secret
        self.offsets = offsets
        self.server_ip = server_ip
        self.local_listen_port = local_listen_port

    def get_totp_port(self, t):
        """
        根据指定时间 t 计算 TOTP 端口
        """
        totp = pyotp.TOTP(self.secret, interval=self.interval)
        otp = totp.at(t)
        port = self.base_port + (int(otp) % self.port_range)
        return port

    def handle_local_connection(self, local_conn):
        """
        当本地应用连接到客户端监听端口后，尝试连接服务端 TOTP 动态端口，并启动双向数据转发。
        """
        print(f"[{time.strftime('%H:%M:%S')}] 本地连接来自 {local_conn.getpeername()}")
        server_conn = None
        for offset in self.offsets:
            try:
                # 使用当前时间加上偏移计算目标端口
                target_time = time.time() + offset
                port = self.get_totp_port(target_time)
                print(f"[{time.strftime('%H:%M:%S')}] 尝试连接服务器 {self.server_ip}:{port}（偏移 {offset}）")
                server_conn = socket.create_connection((self.server_ip, port), timeout=5)
                print(f"[{time.strftime('%H:%M:%S')}] 连接成功：{self.server_ip}:{port}")
                break
            except Exception as e:
                print(f"连接 {self.server_ip}:{port} 失败: {e}")
        if server_conn is None:
            print("无法连接到服务器的 TOTP 端口")
            local_conn.close()
            return

        bidirectional_forward(local_conn, server_conn)
        local_conn.close()
        server_conn.close()
        print(f"[{time.strftime('%H:%M:%S')}] 本地连接关闭，转发结束。")

    def run(self):
        try:
            listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            listener.bind(('0.0.0.0', self.local_listen_port))
            listener.listen(5)
            print(f"服务器 {self.server_ip}")
            print(f"客户端中间件启动，监听 127.0.0.1:{self.local_listen_port}")
        except Exception as e:
            print(f"创建监听 socket 失败: {e}")
            sys.exit(1)

        while True:
            try:
                local_conn, addr = listener.accept()
                local_conn.setblocking(True)
                t = threading.Thread(target=self.handle_local_connection, args=(local_conn,))
                t.start()
            except Exception as e:
                print("接受本地连接失败:", e)

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
    config_file = 'config.toml'
    if len(sys.argv) > 1:
        config_file = sys.argv[1]

    config = load_config(config_file)

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
        server = Server(interval=interval, extend=extend, base_port=base_port, port_range=port_range,
                        secret=secret, offsets=offsets, target_host=host, target_port=port)
        server.run()
    elif mode == 'client':
        # 在客户端模式下，host 为服务端 IP，port 为本地监听端口
        client = Client(interval=interval, extend=extend, base_port=base_port, port_range=port_range,
                        secret=secret, offsets=offsets, server_ip=host, local_listen_port=port)
        client.run()
    else:
        print("无效的 mode 配置，请设置为 'server' 或 'client'")
        sys.exit(1)
