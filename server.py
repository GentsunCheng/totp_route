import selectors
import socket
import time
import pyotp
import threading

# ===================== 配置参数 =====================
interval    = 30                # TOTP 时间窗口（秒）
extend      = 15                # 每个窗口前后延长的秒数（容错时间）
base_port   = 3000              # 动态端口起始值
port_range  = 1000              # 端口映射范围
secret      = 'S3K3TPI5MYA2M67V'# 共享密钥
offsets     = [-15, 0, 15]       # 三个时间偏移
# 目标转发地址（可转发任意协议，对方服务必须能识别数据）
target_host = '127.0.0.1'
target_port = 8888
# =====================================================

# 用来管理各偏移对应的监听 socket：offset -> (sock, valid_start, valid_end, port)
sockets = {}
sel = selectors.DefaultSelector()

def get_window_params(secret, offset):
    """
    根据当前时间加上偏移，计算 TOTP 窗口的起始时间、有效期和映射端口。
    有效期为：窗口开始前 extend 秒 到 窗口结束后 extend 秒
    """
    now = time.time()
    t = now + offset
    window_start = (int(t) // interval) * interval
    valid_start = window_start - extend
    valid_end   = window_start + interval + extend

    totp = pyotp.TOTP(secret, interval=interval)
    otp = totp.at(window_start)
    port_offset = int(otp) % port_range
    port = base_port + port_offset

    return port, valid_start, valid_end

def create_socket(port):
    """创建并返回一个非阻塞的 TCP 监听 socket"""
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.setblocking(False)
    s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    s.bind(('0.0.0.0', port))
    s.listen(5)
    return s

def bidirectional_forward(conn1, conn2):
    """
    双向转发函数，启动两个线程分别转发 conn1->conn2 和 conn2->conn1，
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

def handle_connection(client_conn, offset, listening_port):
    """
    当有客户端连接到 TOTP 监听端口后，建立到目标地址的连接，
    并进行双向数据转发。
    """
    print(f"[{time.strftime('%H:%M:%S')}] 收到来自 {client_conn.getpeername()} 的连接（偏移 {offset}，监听端口 {listening_port}）")
    try:
        target_conn = socket.create_connection((target_host, target_port))
    except Exception as e:
        print(f"连接目标 {target_host}:{target_port} 失败: {e}")
        client_conn.close()
        return

    bidirectional_forward(client_conn, target_conn)
    client_conn.close()
    target_conn.close()
    print(f"[{time.strftime('%H:%M:%S')}] 连接关闭，转发结束。")

print("服务端中间件启动，动态监听 TOTP 端口并转发数据到目标端口...")

if __name__ == '__main__':
    while True:
        now = time.time()
        next_timeout = 5

        # 更新或创建各偏移的监听 socket
        for offset in offsets:
            port, valid_start, valid_end = get_window_params(secret, offset)
            info = sockets.get(offset)
            if info:
                sock, old_valid_start, old_valid_end, old_port = info
                # 如果已超出有效期或端口变更（进入新窗口），则关闭旧 socket
                if now >= old_valid_end or port != old_port:
                    try:
                        sel.unregister(sock)
                    except Exception:
                        pass
                    sock.close()
                    print(f"[{time.strftime('%H:%M:%S')}] 关闭偏移 {offset} 的旧 socket（端口 {old_port}）")
                    del sockets[offset]
                    info = None
            if not info:
                # 如果已到监听时间，则创建新 socket
                if now >= valid_start:
                    try:
                        s = create_socket(port)
                        sel.register(s, selectors.EVENT_READ, data=offset)
                        sockets[offset] = (s, valid_start, valid_end, port)
                        print(f"[{time.strftime('%H:%M:%S')}] 创建监听 socket，偏移 {offset}，端口 {port}，有效期至 {time.strftime('%H:%M:%S', time.localtime(valid_end))}")
                    except Exception as e:
                        print(f"创建端口 {port} 失败: {e}")
                else:
                    next_timeout = min(next_timeout, valid_start - now)

        # 根据各 socket 的剩余有效期，更新下一次超时时间
        for offset, (s, valid_start, valid_end, port) in sockets.items():
            time_to_expiry = valid_end - now
            next_timeout = min(next_timeout, time_to_expiry)

        if not sockets:
            time.sleep(next_timeout)
            continue

        events = sel.select(timeout=next_timeout)
        for key, mask in events:
            s = key.fileobj
            offset = key.data
            try:
                client_conn, addr = s.accept()
                client_conn.setblocking(True)
                # 启动新线程处理该连接（进行数据转发）
                t = threading.Thread(target=handle_connection, args=(client_conn, offset, s.getsockname()[1]))
                t.start()
            except Exception as e:
                print("接受连接时出错:", e)

