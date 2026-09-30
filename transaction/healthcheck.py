"""Healthcheck của container (docker-compose gọi `python healthcheck.py`).

python:3.11-slim không có curl nên healthcheck cũ (`curl -f .../health`) luôn thất bại. Kiểm CẢ HAI cổng vì
container chạy hai tiến trình: REST trả 200 ở /health, và cổng gRPC nhận kết nối (grpc_server.py đã lên).
Thoát 0 khi cả hai ổn, 1 kèm lý do trên stderr nếu không.
"""
import os
import socket
import sys
import urllib.request

REST_PORT = int(os.getenv("TRANSACTION_REST_PORT", "8000"))
GRPC_PORT = int(os.getenv("TRANSACTION_GRPC_PORT", "50051"))


def check(rest_port=REST_PORT, grpc_port=GRPC_PORT, host="127.0.0.1", timeout=5):
    """Trả danh sách lỗi (rỗng = khoẻ)."""
    problems = []
    try:
        with urllib.request.urlopen(f"http://{host}:{rest_port}/health", timeout=timeout) as resp:
            if resp.status != 200:
                problems.append(f"REST /health tra {resp.status}")
    except Exception as e:  # noqa: BLE001 - mọi lỗi kết nối/HTTP đều là "không khoẻ"
        problems.append(f"REST /health loi: {e}")
    try:
        with socket.create_connection((host, grpc_port), timeout=timeout):
            pass
    except OSError as e:
        problems.append(f"gRPC cong {grpc_port} khong nhan ket noi: {e}")
    return problems


if __name__ == "__main__":
    found = check()
    for p in found:
        print(p, file=sys.stderr)
    sys.exit(1 if found else 0)
