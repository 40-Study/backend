"""Entrypoint của container: chạy REST (uvicorn, :8000) và gRPC (grpc_server.py, :50051) cùng lúc.

Vì sao có launcher này: luồng thanh toán của backend Go gọi gRPC ở cổng 50051 của CHÍNH service này (compose
publish 127.0.0.1:50051), nhưng image trước đây chỉ chạy uvicorn nên cổng đó không có tiến trình nào nghe, và
grpc_server.py cũng không nằm trong image. Một container / hai tiến trình (thay vì hai service compose) để giữ
nguyên cổng publish, tên service và biến môi trường hiện có.

Khi MỘT tiến trình dừng (kể cả thoát mã 0, vì service phải chạy mãi) launcher dừng tiến trình còn lại rồi thoát
với mã khác 0, để `restart: unless-stopped` dựng lại cả container thay vì để lại nửa service còn sống mà không ai thấy.
"""
import logging
import os
import signal
import subprocess
import sys
import time

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s run_services: %(message)s")
logger = logging.getLogger("run_services")

REST_PORT = os.getenv("TRANSACTION_REST_PORT", "8000")
# Thời gian chờ tiến trình thoát êm sau SIGTERM trước khi ép kill.
STOP_GRACE_SECONDS = 10


def service_commands():
    """Lệnh của từng tiến trình. Dùng sys.executable -m uvicorn để không phụ thuộc PATH của pip --user."""
    return {
        "rest": [sys.executable, "-m", "uvicorn", "api:app", "--host", "0.0.0.0", "--port", REST_PORT],
        "grpc": [sys.executable, "grpc_server.py"],
    }


def _stop(procs, grace):
    for p in procs.values():
        if p.poll() is None:
            p.terminate()
    deadline = time.monotonic() + grace
    for p in procs.values():
        try:
            p.wait(timeout=max(0.0, deadline - time.monotonic()))
        except subprocess.TimeoutExpired:
            p.kill()
            p.wait()


def run_all(commands, poll_interval=0.5, grace=STOP_GRACE_SECONDS):
    """Chạy các lệnh song song; trả mã thoát của launcher (0 chỉ khi bị yêu cầu dừng, còn lại khác 0)."""
    procs = {name: subprocess.Popen(cmd) for name, cmd in commands.items()}
    stop_requested = []

    def on_signal(signum, _frame):
        stop_requested.append(signum)

    previous = {sig: signal.signal(sig, on_signal) for sig in (signal.SIGTERM, signal.SIGINT)}

    exit_code = 0
    try:
        while not stop_requested:
            for name, p in procs.items():
                rc = p.poll()
                if rc is not None:
                    logger.error("tien trinh %s da dung (ma thoat %s), dung ca container", name, rc)
                    exit_code = rc or 1
                    stop_requested.append(name)
                    break
            else:
                time.sleep(poll_interval)
    finally:
        _stop(procs, grace)
        for sig, handler in previous.items():
            signal.signal(sig, handler)
    return exit_code


if __name__ == "__main__":
    sys.exit(run_all(service_commands()))
