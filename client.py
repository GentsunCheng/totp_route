import socket
import threading
import time
import pyotp

# ===================== 配置参数 =====================
local_listen_port = 9000     # 客户端本地监听端口
server_ip         = '127.0.0.1'  # 服务端 IP，请替换为实际地址
interval          = 30       # TOTP 时间窗口（秒）
extend            = 15       # TOTP 延长时间（秒）
base_port         = 3000     # 服务端动态端口起始值
port_range        = 1000     # 端口映射范围
secret            = 'S3K3TPI5MYA2M67V'  # 共享密钥
offsets           = [-15, 0, 15] # 尝试的时间偏移
# =====================================================

def get_totp_port(secret, t, base_port=3000, port_range=1000):
    totp = pyotp.TOTP(secret, interval=interval)
    otp = totp.at(t)
    port = base_port + (int(otp) % port_range)
    return port

def bidirectional_forward(conn1, conn2):
    """
    双向转发函数，启动两个线程分别转发 conn1->conn2 与 conn2->conn1
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

def handle_local_connection(local_conn):
    """
    当本地应用连接到客户端监听端口后，尝试连接服务端 TOTP 动态端口，
    并启动双向数据转发。
    """
    print(f"[{time.strftime('%H:%M:%S')}] 本地连接来自 {local_conn.getpeername()}")
    server_conn = None
    for offset in offsets:
        try:
            # 使用当前时间加上偏移计算目标端口
            port = get_totp_port(secret, time.time() + offset, base_port, port_range)
            print(f"[{time.strftime('%H:%M:%S')}] 尝试连接服务器 {server_ip}:{port}（偏移 {offset}）")
            server_conn = socket.create_connection((server_ip, port), timeout=5)
            print(f"[{time.strftime('%H:%M:%S')}] 连接成功：{server_ip}:{port}")
            break
        except Exception as e:
            print(f"连接 {server_ip}:{port} 失败: {e}")
    if server_conn is None:
        print("无法连接到服务器的 TOTP 端口")
        local_conn.close()
        return

    bidirectional_forward(local_conn, server_conn)
    local_conn.close()
    server_conn.close()
    print(f"[{time.strftime('%H:%M:%S')}] 本地连接关闭，转发结束。")

def main():
    listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    listener.bind(('0.0.0.0', local_listen_port))
    listener.listen(5)
    print(f"客户端中间件启动，监听本地端口 {local_listen_port}，转发数据至服务器 {server_ip}")
    while True:
        try:
            local_conn, addr = listener.accept()
            local_conn.setblocking(True)
            t = threading.Thread(target=handle_local_connection, args=(local_conn,))
            t.start()
        except Exception as e:
            print("接受本地连接失败:", e)

if __name__ == '__main__':
    main()

