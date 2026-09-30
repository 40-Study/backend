"""S6: kiểm tra xác thực token dùng chung của service giao dịch (REST api.py và gRPC grpc_server.py).

Chạy: cd transaction && python -m pytest -q
Không cần MB Bank thật: mbbank và mã sinh từ proto được thay bằng bản giả trước khi import.
"""
import sys
import types
from concurrent import futures

import grpc
import pytest

TOKEN = "s6-shared-secret"


# mbbank giả nằm ở conftest.py (dùng chung với test_health_and_launcher.py).

QUERY = "from_date=00-00-00-01-01-2024&to_date=00-00-00-02-01-2024"
ROUTES = [
    "/transactions?" + QUERY,
    "/transactions/count?" + QUERY,
    "/transactions/check-pin?pin=ABCDEFGHIJKLMNO&" + QUERY,
]


@pytest.fixture()
def client(monkeypatch):
    from fastapi.testclient import TestClient
    import api

    return TestClient(api.app), monkeypatch


@pytest.mark.parametrize("path", ROUTES)
def test_rest_without_token_is_rejected(client, path):
    c, monkeypatch = client
    monkeypatch.setenv("TRANSACTION_SERVICE_TOKEN", TOKEN)
    assert c.get(path).status_code == 401


@pytest.mark.parametrize("path", ROUTES)
def test_rest_wrong_token_is_rejected(client, path):
    c, monkeypatch = client
    monkeypatch.setenv("TRANSACTION_SERVICE_TOKEN", TOKEN)
    assert c.get(path, headers={"X-Transaction-Token": "nope"}).status_code == 401


@pytest.mark.parametrize("path", ROUTES)
def test_rest_right_token_is_accepted(client, path):
    c, monkeypatch = client
    monkeypatch.setenv("TRANSACTION_SERVICE_TOKEN", TOKEN)
    assert c.get(path, headers={"X-Transaction-Token": TOKEN}).status_code == 200


@pytest.mark.parametrize("path", ROUTES)
def test_rest_fails_closed_when_token_not_configured(client, path):
    c, monkeypatch = client
    monkeypatch.delenv("TRANSACTION_SERVICE_TOKEN", raising=False)
    # kể cả gửi header rỗng hay đoán "" cũng không được qua
    assert c.get(path).status_code == 503
    assert c.get(path, headers={"X-Transaction-Token": ""}).status_code == 503


# ── gRPC ────────────────────────────────────────────────────────────────────


def _load_grpc_server():
    """grpc_server.py import transaction_pb2* (mã sinh, không nằm trong repo): thay bằng bản giả gọn."""
    pb2 = types.ModuleType("transaction_pb2")
    pb2_grpc = types.ModuleType("transaction_pb2_grpc")
    pb2_grpc.TransactionServiceServicer = object
    pb2_grpc.add_TransactionServiceServicer_to_server = lambda servicer, server: None
    sys.modules["transaction_pb2"] = pb2
    sys.modules["transaction_pb2_grpc"] = pb2_grpc
    sys.modules.pop("grpc_server", None)
    import grpc_server

    return grpc_server


def _call_with_interceptor(token_on_server, metadata):
    """Dựng server thật với interceptor và gọi 1 RPC unary bằng handler generic."""
    grpc_server = _load_grpc_server()

    def echo(request, context):
        return b"ok"

    handler = grpc.method_handlers_generic_handler(
        "test.Echo", {"Ping": grpc.unary_unary_rpc_method_handler(echo)}
    )
    interceptors = [grpc_server.TokenAuthInterceptor(token_on_server)] if token_on_server else []
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=2), interceptors=interceptors)
    server.add_generic_rpc_handlers((handler,))
    port = server.add_insecure_port("127.0.0.1:0")
    server.start()
    try:
        with grpc.insecure_channel(f"127.0.0.1:{port}") as ch:
            ping = ch.unary_unary("/test.Echo/Ping")
            return ping(b"x", metadata=metadata, timeout=5)
    finally:
        server.stop(0)


def test_grpc_missing_token_is_rejected():
    with pytest.raises(grpc.RpcError) as e:
        _call_with_interceptor(TOKEN, None)
    assert e.value.code() == grpc.StatusCode.UNAUTHENTICATED


def test_grpc_wrong_token_is_rejected():
    with pytest.raises(grpc.RpcError) as e:
        _call_with_interceptor(TOKEN, (("x-transaction-token", "nope"),))
    assert e.value.code() == grpc.StatusCode.UNAUTHENTICATED


def test_grpc_right_token_is_accepted():
    assert _call_with_interceptor(TOKEN, (("x-transaction-token", TOKEN),)) == b"ok"


# ── Khởi động fail-closed (M3) ──────────────────────────────────────────────


def test_rest_refuses_to_start_without_token(monkeypatch):
    from fastapi.testclient import TestClient
    import api
    import service_token

    monkeypatch.delenv("TRANSACTION_SERVICE_TOKEN", raising=False)
    monkeypatch.delenv("ALLOW_INSECURE_TRANSACTIONS", raising=False)
    with pytest.raises(service_token.MissingServiceTokenError):
        with TestClient(api.app):
            pass


def test_rest_starts_with_token(monkeypatch):
    from fastapi.testclient import TestClient
    import api

    monkeypatch.setenv("TRANSACTION_SERVICE_TOKEN", TOKEN)
    monkeypatch.delenv("ALLOW_INSECURE_TRANSACTIONS", raising=False)
    with TestClient(api.app) as c:
        assert c.get("/transactions?" + QUERY, headers={"X-Transaction-Token": TOKEN}).status_code == 200


def test_rest_starts_only_with_explicit_dev_flag(monkeypatch):
    from fastapi.testclient import TestClient
    import api

    monkeypatch.delenv("TRANSACTION_SERVICE_TOKEN", raising=False)
    monkeypatch.setenv("ALLOW_INSECURE_TRANSACTIONS", "1")
    with TestClient(api.app) as c:
        assert c.get("/transactions?" + QUERY).status_code == 200  # dev: không xác thực


@pytest.mark.parametrize("value", ["", "0", "true", "yes", "1 "])
def test_dev_flag_must_be_exactly_1(monkeypatch, value):
    import service_token

    monkeypatch.delenv("TRANSACTION_SERVICE_TOKEN", raising=False)
    monkeypatch.setenv("ALLOW_INSECURE_TRANSACTIONS", value)
    with pytest.raises(service_token.MissingServiceTokenError):
        service_token.load_service_token()


def test_grpc_refuses_to_start_without_token(monkeypatch):
    import service_token

    grpc_server = _load_grpc_server()
    monkeypatch.delenv("TRANSACTION_SERVICE_TOKEN", raising=False)
    monkeypatch.delenv("ALLOW_INSECURE_TRANSACTIONS", raising=False)
    with pytest.raises(service_token.MissingServiceTokenError):
        grpc_server.serve(port=0, token="")


def test_grpc_starts_with_token_and_with_dev_flag(monkeypatch):
    grpc_server = _load_grpc_server()
    monkeypatch.delenv("ALLOW_INSECURE_TRANSACTIONS", raising=False)
    s = grpc_server.serve(port=0, token=TOKEN)
    s.stop(0)
    monkeypatch.delenv("TRANSACTION_SERVICE_TOKEN", raising=False)
    monkeypatch.setenv("ALLOW_INSECURE_TRANSACTIONS", "1")
    s = grpc_server.serve(port=0, token="")
    s.stop(0)