"""S6: cấu hình token dùng chung của service giao dịch, dùng chung cho REST (api.py) và gRPC (grpc_server.py).

Fail-closed: thiếu TRANSACTION_SERVICE_TOKEN thì service KHÔNG khởi động. Cách duy nhất để chạy không token là đặt
tường minh ALLOW_INSECURE_TRANSACTIONS=1 (chỉ dành cho máy dev); khi đó service khởi động kèm cảnh báo lớn.
"""
import logging
import os

logger = logging.getLogger(__name__)

TOKEN_ENV = "TRANSACTION_SERVICE_TOKEN"
INSECURE_ENV = "ALLOW_INSECURE_TRANSACTIONS"


class MissingServiceTokenError(RuntimeError):
    """Thiếu token mà không có cờ cho phép chạy không xác thực."""


def insecure_allowed(env=None) -> bool:
    env = os.environ if env is None else env
    return env.get(INSECURE_ENV, "") == "1"


def load_service_token(env=None) -> str:
    """Trả token đã cấu hình. Không có token: raise, trừ khi ALLOW_INSECURE_TRANSACTIONS=1 (khi đó trả "")."""
    env = os.environ if env is None else env
    token = env.get(TOKEN_ENV, "")
    if token:
        return token
    if insecure_allowed(env):
        logger.warning(
            "%s=1 va %s trong: service giao dich chay KHONG xac thuc. Chi dung cho may dev, KHONG dung o production.",
            INSECURE_ENV,
            TOKEN_ENV,
        )
        return ""
    raise MissingServiceTokenError(
        f"{TOKEN_ENV} chua duoc cau hinh: tu choi khoi dong service giao dich. "
        f"Dat {TOKEN_ENV} (cung gia tri voi backend Go), hoac chi cho may dev dat {INSECURE_ENV}=1."
    )