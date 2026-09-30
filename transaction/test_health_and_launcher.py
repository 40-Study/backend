"""Lane P: /health của REST, launcher chạy REST + gRPC trong một container, và healthcheck của container.

Chạy: cd transaction && python -m pytest -q
"""
import http.server
import socket
import sys
import threading
import time

import pytest

TOKEN = "lane-p-secret"
QUERY = "from_date=00-00-00-01-01-2024&to_date=00-00-00-02-01-2024"


# ── /health ────────────────────────────────────────────────────────────────


def test_health_needs_no_token_and_leaks_nothing(monkeypatch):
    from fastapi.testclient import TestClient
    import api

    monkeypatch.setenv("TRANSACTION_SERVICE_TOKEN", TOKEN)
    with TestClient(api.app) as c:
        r = c.get("/health")
    assert r.status_code == 200
    # Chỉ một cờ tĩnh: không số tài khoản, không tên đăng nhập MB, không cấu hình.
    assert r.json() == {"status": "ok"}


def test_health_stays_open_but_transactions_still_need_token(monkeypatch):
    from fastapi.testclient import TestClient
    import api

    monkeypatch.setenv("TRANSACTION_SERVICE_TOKEN", TOKEN)
    with TestClient(api.app) as c:
        assert c.get("/health").status_code == 200
        assert c.get("/transactions?" + QUERY).status_code == 401
        assert c.get("/transactions?" + QUERY, headers={"X-Transaction-Token": TOKEN}).status_code == 200


# ── launcher ───────────────────────────────────────────────────────────────


def _py(code):
    return [sys.executable, "-c", code]


def test_launcher_stops_everything_when_one_process_dies(tmp_path):
    import run_services

    marker = tmp_path / "survivor.txt"
    survivor = f"import time; time.sleep(3); open({str(marker)!r}, 'w').write('alive')"
    started = time.monotonic()
    code = run_services.run_all(
        {"long": _py(survivor), "short": _py("import sys; time = 0; sys.exit(3)")},
        poll_interval=0.05,
        grace=5,
    )
    assert code == 3  # mã lỗi của tiến trình chết được chuyển ra ngoài để compose thấy container lỗi
    assert time.monotonic() - started < 20  # không ngồi chờ tiến trình còn lại chạy hết
    # Tiến trình còn lại phải bị dừng thật, không chỉ launcher trả về: nếu nó còn sống thì sau 3 giây sẽ ghi marker.
    time.sleep(4)
    assert not marker.exists(), "tiến trình còn lại vẫn chạy sau khi launcher thoát"


def test_launcher_treats_a_clean_exit_as_failure():
    import run_services

    code = run_services.run_all(
        {"long": _py("import time; time.sleep(25)"), "short": _py("import sys; sys.exit(0)")},
        poll_interval=0.05,
        grace=5,
    )
    assert code == 1  # service phải chạy mãi: thoát mã 0 cũng là chết


def test_launcher_commands_start_both_rest_and_grpc():
    import run_services

    cmds = run_services.service_commands()
    assert set(cmds) == {"rest", "grpc"}
    assert "uvicorn" in cmds["rest"] and "api:app" in cmds["rest"]
    assert cmds["grpc"][-1] == "grpc_server.py"


# ── healthcheck ────────────────────────────────────────────────────────────


class _Handler(http.server.BaseHTTPRequestHandler):
    status = 200

    def do_GET(self):
        self.send_response(type(self).status)
        self.end_headers()

    def log_message(self, *_):
        pass


def _free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


@pytest.fixture()
def rest_and_grpc():
    """Một HTTP server giả (/health) và một socket đang listen giả làm cổng gRPC."""
    _Handler.status = 200
    httpd = http.server.ThreadingHTTPServer(("127.0.0.1", 0), _Handler)
    threading.Thread(target=httpd.serve_forever, daemon=True).start()
    grpc_sock = socket.socket()
    grpc_sock.bind(("127.0.0.1", 0))
    grpc_sock.listen()
    try:
        yield httpd.server_address[1], grpc_sock.getsockname()[1]
    finally:
        httpd.shutdown()
        grpc_sock.close()


def test_healthcheck_passes_when_both_ports_are_up(rest_and_grpc):
    import healthcheck

    rest, grpc_port = rest_and_grpc
    assert healthcheck.check(rest_port=rest, grpc_port=grpc_port, timeout=2) == []


def test_healthcheck_fails_when_grpc_port_is_closed(rest_and_grpc):
    import healthcheck

    rest, _ = rest_and_grpc
    problems = healthcheck.check(rest_port=rest, grpc_port=_free_port(), timeout=2)
    assert len(problems) == 1 and "gRPC" in problems[0]


def test_healthcheck_fails_when_rest_is_down_or_unhealthy(rest_and_grpc):
    import healthcheck

    rest, grpc_port = rest_and_grpc
    _Handler.status = 503
    assert any("REST" in p for p in healthcheck.check(rest_port=rest, grpc_port=grpc_port, timeout=2))
    assert any("REST" in p for p in healthcheck.check(rest_port=_free_port(), grpc_port=grpc_port, timeout=2))
