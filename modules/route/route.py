#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import selectors
import socket
import time
import threading
import pyotp
import toml
import sys

def bidirectional_forward_tcp(conn1, conn2):
    """
    TCP 双向转发：启动两个线程分别转发 conn1->conn2 与 conn2->conn1，
    直到任一端断开连接。
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
    服务端中间件类：
      - 根据 TOTP 算法动态监听端口（使用 protocol）
      - 当有客户端接入时，通过 protocol 将数据转发至目标地址（目标程序）
    """
    def __init__(self, interval, extend, base_port, port_range, secret, offsets,
                 target_host, target_port, protocol):
        self.interval = interval
        self.extend = extend
        self.base_port = base_port
        self.port_range = port_range
        self.secret = secret
        self.offsets = offsets
        self.target_host = target_host
        self.target_port = target_port

        self.protocol = protocol.lower()
        if self.protocol not in ["tcp", "udp"]:
            raise ValueError("protocol 参数只支持 tcp 或 udp")

        self.sockets = {}  # offset -> (sock, valid_start, valid_end, port)
        self.sel = selectors.DefaultSelector()

    def get_window_params(self, offset):
        """
        根据当前时间加上偏移，计算 TOTP 窗口起始时间、有效期及映射端口  
        有效期：窗口开始前 extend 秒 到窗口结束后 extend 秒
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

    def create_tcp_socket(self, port):
        """创建 TCP 监听 socket（非阻塞）"""
        s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        s.setblocking(False)
        s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        s.bind(('0.0.0.0', port))
        s.listen(5)
        return s

    def create_udp_socket(self, port):
        """创建 UDP socket（非阻塞）"""
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.setblocking(False)
        s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        s.bind(('0.0.0.0', port))
        return s

    def handle_tcp_connection(self, client_conn, offset, listening_port):
        """
        TCP 处理函数：客户端通过 TOTP 动态端口连接后，
        建立到目标地址的 TCP 连接，并启动双向转发。
        """
        print(f"[{time.strftime('%H:%M:%S')}] 收到 {client_conn.getpeername()} 的连接（偏移 {offset}，端口 {listening_port}）")
        try:
            target_conn = socket.create_connection((self.target_host, self.target_port))
        except Exception as e:
            print(f"连接目标 {self.target_host}:{self.target_port} 失败: {e}")
            client_conn.close()
            return

        bidirectional_forward_tcp(client_conn, target_conn)
        client_conn.close()
        target_conn.close()
        print(f"[{time.strftime('%H:%M:%S')}] 连接关闭，转发结束。")

    def handle_udp_packet(self, sock, data, client_addr, offset, listening_port):
        """
        UDP 处理函数：收到 UDP 数据包后，
        通过 UDP 将数据发送到目标地址，再将目标响应转发回客户端。
        """
        print(f"[{time.strftime('%H:%M:%S')}] 收到来自 {client_addr} 的 UDP 数据包（偏移 {offset}，端口 {listening_port}）")
        try:
            target_sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            target_sock.settimeout(5)
            target_sock.sendto(data, (self.target_host, self.target_port))
            response, _ = target_sock.recvfrom(4096)
            sock.sendto(response, client_addr)
            print(f"[{time.strftime('%H:%M:%S')}] UDP 转发成功")
        except Exception as e:
            print(f"UDP 转发出错: {e}")

    def run(self):
        if self.protocol == "tcp":
            self.run_tcp()
        elif self.protocol == "udp":
            self.run_udp()

    def run_tcp(self):
        print("服务端中间件启动（TCP），动态监听 TOTP 端口并转发数据到目标...")
        while True:
            now = time.time()
            next_timeout = 5

            # 更新或创建各偏移对应的监听 socket
            for offset in self.offsets:
                port, valid_start, valid_end = self.get_window_params(offset)
                info = self.sockets.get(offset)
                if info:
                    sock, old_valid_start, old_valid_end, old_port = info
                    if now >= old_valid_end or port != old_port:
                        try:
                            self.sel.unregister(sock)
                        except Exception:
                            pass
                        sock.close()
                        print(f"[{time.strftime('%H:%M:%S')}] 关闭旧 socket（偏移 {offset}，端口 {old_port}）")
                        del self.sockets[offset]
                        info = None
                if not info:
                    if now >= valid_start:
                        try:
                            s = self.create_tcp_socket(port)
                            self.sel.register(s, selectors.EVENT_READ, data=offset)
                            self.sockets[offset] = (s, valid_start, valid_end, port)
                            print(f"[{time.strftime('%H:%M:%S')}] 创建监听 socket（偏移 {offset}，端口 {port}），有效期至 {time.strftime('%H:%M:%S', time.localtime(valid_end))}")
                        except Exception as e:
                            print(f"创建端口 {port} 失败: {e}")
                    else:
                        next_timeout = min(next_timeout, valid_start - now)

            for offset, (s, valid_start, valid_end, port) in self.sockets.items():
                next_timeout = min(next_timeout, valid_end - now)

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
                    threading.Thread(target=self.handle_tcp_connection, args=(client_conn, offset, s.getsockname()[1])).start()
                except Exception as e:
                    print("接受连接时出错:", e)

    def run_udp(self):
        print("服务端中间件启动（UDP），动态监听 TOTP 端口并转发数据到目标...")
        sel_udp = selectors.DefaultSelector()
        while True:
            now = time.time()
            next_timeout = 5

            # 更新或创建各偏移对应的 UDP socket
            for offset in self.offsets:
                port, valid_start, valid_end = self.get_window_params(offset)
                info = self.sockets.get(offset)
                if info:
                    sock, old_valid_start, old_valid_end, old_port = info
                    if now >= old_valid_end or port != old_port:
                        try:
                            sel_udp.unregister(sock)
                        except Exception:
                            pass
                        sock.close()
                        print(f"[{time.strftime('%H:%M:%S')}] 关闭旧 UDP socket（偏移 {offset}，端口 {old_port}）")
                        del self.sockets[offset]
                        info = None
                if not info:
                    if now >= valid_start:
                        try:
                            s = self.create_udp_socket(port)
                            sel_udp.register(s, selectors.EVENT_READ, data=offset)
                            self.sockets[offset] = (s, valid_start, valid_end, port)
                            print(f"[{time.strftime('%H:%M:%S')}] 创建 UDP socket（偏移 {offset}，端口 {port}），有效期至 {time.strftime('%H:%M:%S', time.localtime(valid_end))}")
                        except Exception as e:
                            print(f"创建 UDP 端口 {port} 失败: {e}")
                    else:
                        next_timeout = min(next_timeout, valid_start - now)

            for offset, (s, valid_start, valid_end, port) in self.sockets.items():
                next_timeout = min(next_timeout, valid_end - now)

            events = sel_udp.select(timeout=next_timeout)
            for key, mask in events:
                s = key.fileobj
                offset = key.data
                try:
                    data, addr = s.recvfrom(4096)
                    if data:
                        threading.Thread(target=self.handle_udp_packet, args=(s, data, addr, offset, s.getsockname()[1])).start()
                except Exception as e:
                    print("处理 UDP 数据包时出错:", e)

class Client:
    """
    客户端中间件类：
      - 在本地使用 protocol 监听（客户端接入协议）
      - 当本地应用连接时，通过 protocol（TOTP 动态端口）连接至服务器，
        然后进行数据转发
    """
    def __init__(self, interval, extend, base_port, port_range, secret, offsets,
                 server_ip, local_listen_port, protocol):
        self.interval = interval
        self.extend = extend
        self.base_port = base_port
        self.port_range = port_range
        self.secret = secret
        self.offsets = offsets
        self.server_ip = server_ip
        self.local_listen_port = local_listen_port

        self.protocol = protocol.lower()
        if self.protocol not in ["tcp", "udp"]:
            raise ValueError("protocol 参数只支持 tcp 或 udp")

    def get_totp_port(self, t):
        totp = pyotp.TOTP(self.secret, interval=self.interval)
        otp = totp.at(t)
        port_offset = int(otp) % self.port_range
        return self.base_port + port_offset

    def handle_tcp_connection(self, local_conn):
        """
        TCP 处理函数：本地应用连接后，
        客户端尝试通过 protocol 连接服务器的 TOTP 动态端口，
        并进行双向转发。
        """
        print(f"[{time.strftime('%H:%M:%S')}] 本地连接来自 {local_conn.getpeername()}")
        server_conn = None
        for offset in self.offsets:
            try:
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
        bidirectional_forward_tcp(local_conn, server_conn)
        local_conn.close()
        server_conn.close()
        print(f"[{time.strftime('%H:%M:%S')}] 本地连接关闭，转发结束。")

    def handle_udp_packet(self, local_sock, data, client_addr):
        """
        UDP 处理函数：收到本地 UDP 数据包后，
        客户端尝试通过 protocol 向服务器发送数据，
        并将服务器响应转发回本地。
        """
        print(f"[{time.strftime('%H:%M:%S')}] 本地 UDP 数据包来自 {client_addr}")
        for offset in self.offsets:
            try:
                target_time = time.time() + offset
                port = self.get_totp_port(target_time)
                print(f"[{time.strftime('%H:%M:%S')}] 尝试通过 UDP 连接服务器 {self.server_ip}:{port}（偏移 {offset}）")
                server_sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
                server_sock.settimeout(5)
                server_sock.sendto(data, (self.server_ip, port))
                response, _ = server_sock.recvfrom(4096)
                local_sock.sendto(response, client_addr)
                print(f"[{time.strftime('%H:%M:%S')}] UDP 转发成功，使用偏移 {offset}")
                return
            except Exception as e:
                print(f"连接服务器 {self.server_ip}:{port} 失败: {e}")
        print("无法通过 UDP 连接到服务器的 TOTP 端口")

    def run(self):
        if self.protocol == "tcp":
            self.run_tcp()
        elif self.protocol == "udp":
            self.run_udp()

    def run_tcp(self):
        try:
            listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            listener.bind(('0.0.0.0', self.local_listen_port))
            listener.listen(5)
            print(f"客户端中间件启动（TCP），监听本地端口 {self.local_listen_port}，转发数据至服务器 {self.server_ip}")
        except Exception as e:
            print(f"创建监听 socket 失败: {e}")
            sys.exit(1)
        while True:
            try:
                local_conn, addr = listener.accept()
                local_conn.setblocking(True)
                threading.Thread(target=self.handle_tcp_connection, args=(local_conn,)).start()
            except Exception as e:
                print("接受本地连接失败:", e)

    def run_udp(self):
        try:
            local_sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            local_sock.bind(('0.0.0.0', self.local_listen_port))
            local_sock.setblocking(False)
            print(f"客户端中间件启动（UDP），监听本地端口 {self.local_listen_port}，转发数据至服务器 {self.server_ip}")
        except Exception as e:
            print(f"创建本地 UDP 监听 socket 失败: {e}")
            sys.exit(1)
        sel_udp = selectors.DefaultSelector()
        sel_udp.register(local_sock, selectors.EVENT_READ)
        while True:
            events = sel_udp.select(timeout=5)
            for key, mask in events:
                sock = key.fileobj
                try:
                    data, addr = sock.recvfrom(4096)
                    if data:
                        threading.Thread(target=self.handle_udp_packet, args=(local_sock, data, addr)).start()
                except Exception as e:
                    print("接收本地 UDP 数据失败:", e)

def load_config(config_file):
    """
    从 toml 文件加载配置参数  
    示例配置（config.toml）：
        interval = 30
        extend = 15
        base_port = 3000
        port_range = 1000
        secret = "S3K3TPI5MYA2M67V"
        offsets = [-15, 0, 15]
        host = "127.0.0.1"      # 服务端模式下为目标地址；客户端模式下为服务器 IP
        port = 8080             # 服务端模式下为目标端口；客户端模式下为本地监听端口
        mode = "server"         # server 或 client
        protocol = "tcp"        # 或 "udp"
    """
    try:
        config = toml.load(config_file)
        return config
    except Exception as e:
        print(f"加载配置文件 {config_file} 失败: {e}")
        sys.exit(1)

if __name__ == '__main__':
    # 默认配置文件为 config.toml，也可通过命令行参数指定
    config_file = 'config.toml'
    if len(sys.argv) > 1:
        config_file = sys.argv[1]
    config = load_config(config_file)

    interval   = config.get('interval', 30)
    extend     = config.get('extend', 15)
    base_port  = config.get('base_port', 3000)
    port_range = config.get('port_range', 1000)
    secret     = config.get('secret', '')
    offsets    = config.get('offsets', [-15, 0, 15])
    host       = config.get('host', '127.0.0.1')
    port       = config.get('port', 8080)
    mode       = config.get('mode', 'server').lower()

    protocol   = config.get('protocol', 'tcp').lower()

    if mode == 'server':
        server = Server(interval=interval, extend=extend, base_port=base_port, port_range=port_range,
                        secret=secret, offsets=offsets, target_host=host, target_port=port,
                        protocol=protocol)
        server.run()
    elif mode == 'client':
        client = Client(interval=interval, extend=extend, base_port=base_port, port_range=port_range,
                        secret=secret, offsets=offsets, server_ip=host, local_listen_port=port,
                        protocol=protocol)
        client.run()
    else:
        print("无效的 mode 配置，请设置为 'server' 或 'client'")
        sys.exit(1)
