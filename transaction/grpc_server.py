import decimal
import grpc
import hmac
import time
from concurrent import futures
import datetime
import logging
import os
import mbbank
import service_token

import transaction_pb2
import transaction_pb2_grpc

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

USERNAME = ""
PASSWORD = ""
ACCOUNT_NO = ""

# S6: xác thực giữa backend Go và service này bằng token dùng chung trong metadata gRPC
# "x-transaction-token" (cùng biến TRANSACTION_SERVICE_TOKEN với api.py và backend Go).
# Luồng mua xu/thanh toán gọi CheckTransaction qua đây nên KHÔNG được làm hỏng: khi chưa cấu hình token,
# server vẫn nhận mọi request (như trước) nhưng ghi cảnh báo lớn lúc khởi động; khi đã cấu hình, request
# thiếu/sai token bị từ chối UNAUTHENTICATED. Bật token: đặt cùng giá trị ở CẢ HAI phía (Go trước, rồi Python).
TOKEN_METADATA_KEY = "x-transaction-token"


class TokenAuthInterceptor(grpc.ServerInterceptor):
    """Từ chối RPC không kèm đúng token trong metadata."""

    def __init__(self, token: str):
        self._token = token.encode("utf-8")

        def deny(request, context):
            context.abort(grpc.StatusCode.UNAUTHENTICATED, "invalid or missing service token")

        self._deny_handler = grpc.unary_unary_rpc_method_handler(deny)

    def intercept_service(self, continuation, handler_call_details):
        provided = ""
        for key, value in handler_call_details.invocation_metadata or ():
            if key == TOKEN_METADATA_KEY:
                provided = value
                break
        if hmac.compare_digest(str(provided).encode("utf-8"), self._token):
            return continuation(handler_call_details)
        return self._deny_handler


def parse_timestamp(ts):
    """Convert unix timestamp to datetime"""
    return datetime.datetime.fromtimestamp(ts)


def get_mbbank_client():
    """Create MB Bank client"""
    return mbbank.MBBank(username=USERNAME, password=PASSWORD)


def _mentions_payment_code(trans, payment_code):
    """Mã thanh toán có nằm trong nội dung chuyển khoản (description hoặc addDescription) không."""
    description = getattr(trans, 'description', '') or ''
    add_description = getattr(trans, 'addDescription', '') or ''
    return payment_code in description or payment_code in add_description


def _is_credit(trans):
    """Giao dịch có ghi có (tiền VÀO tài khoản) không.

    Bắt buộc khi trả MỌI giao dịch khớp: nội dung chuyển khoản hoàn tiền của admin cũng chứa mã đơn,
    nhưng là giao dịch ghi nợ (credit 0). Nếu coi là tiền khách chuyển thì backend Go gắn cờ "cần hoàn
    tiền" cho chính khoản hoàn đó, và đơn không bao giờ hết cờ. Không đọc được số tiền thì GIỮ giao
    dịch lại (để Go tự đối chiếu số tiền), không giấu một khoản có thể là tiền thật.
    """
    raw = str(getattr(trans, 'creditAmount', '') or '').strip().replace(',', '')
    if raw == '':
        return False
    try:
        return decimal.Decimal(raw) > 0
    except decimal.InvalidOperation:
        return True


def _to_matched_transaction(trans):
    return transaction_pb2.MatchedTransaction(
        transaction_id=getattr(trans, 'refNo', '') or '',
        amount=getattr(trans, 'creditAmount', '0') or '0',
        currency="VND",
        description=getattr(trans, 'description', '') or '',
        transaction_date=getattr(trans, 'transactionDate', '') or '',
    )


class TransactionServicer(transaction_pb2_grpc.TransactionServiceServicer):
    """gRPC Service implementation for MB Bank transactions"""

    def CheckTransaction(self, request, context):
        """Check if a transaction exists with given payment code"""
        payment_code = request.payment_code
        from_ts = request.from_timestamp
        to_ts = request.to_timestamp

        logger.info(f"Checking transaction for payment code: {payment_code}")

        try:
            from_dt = parse_timestamp(from_ts)
            to_dt = parse_timestamp(to_ts)

            if from_dt > to_dt:
                return transaction_pb2.CheckTransactionResponse(
                    found=False,
                    status="error",
                    error_message="from_timestamp must be before to_timestamp"
                )

            mb = get_mbbank_client()
            transactions = mb.getTransactionAccountHistory(
                accountNo=ACCOUNT_NO,
                from_date=from_dt,
                to_date=to_dt
            )

            # Khách có thể chuyển NHIỀU lần vào cùng một mã đơn: trả MỌI giao dịch ghi có khớp mã (field
            # `transactions`), không dừng ở giao dịch đầu. Trước đây dừng ở giao dịch đầu nên lần chuyển
            # thứ hai không bao giờ tới được backend Go và cờ hoàn tiền muộn không bật.
            history = getattr(transactions, 'transactionHistoryList', None) or []
            matches = [
                trans for trans in history
                if _mentions_payment_code(trans, payment_code) and _is_credit(trans)
            ]
            if matches:
                first = _to_matched_transaction(matches[0])
                return transaction_pb2.CheckTransactionResponse(
                    # Field 2-6 giữ nghĩa cũ = giao dịch khớp đầu tiên (client cũ vẫn chạy đúng).
                    found=True,
                    transaction_id=first.transaction_id,
                    amount=first.amount,
                    currency=first.currency,
                    description=first.description,
                    transaction_date=first.transaction_date,
                    status="success",
                    transactions=[_to_matched_transaction(trans) for trans in matches],
                )

            # No transaction found
            return transaction_pb2.CheckTransactionResponse(
                found=False,
                status="not_found",
                error_message="No transaction found with this payment code"
            )

        except Exception as e:
            logger.error(f"Error checking transaction: {str(e)}")
            return transaction_pb2.CheckTransactionResponse(
                found=False,
                status="error",
                error_message=str(e)
            )

    def GetTransactions(self, request, context):
        """Get all transactions in date range"""
        from_ts = request.from_timestamp
        to_ts = request.to_timestamp
        limit = request.limit if request.limit > 0 else 100

        logger.info(f"Getting transactions from {from_ts} to {to_ts}")

        try:
            from_dt = parse_timestamp(from_ts)
            to_dt = parse_timestamp(to_ts)

            mb = get_mbbank_client()
            transactions = mb.getTransactionAccountHistory(
                accountNo=ACCOUNT_NO,
                from_date=from_dt,
                to_date=to_dt
            )

            result_transactions = []
            count = 0

            if hasattr(transactions, 'transactionHistoryList') and transactions.transactionHistoryList:
                for trans in transactions.transactionHistoryList[:limit]:
                    result_transactions.append(transaction_pb2.TransactionData(
                        posting_date=getattr(trans, 'postingDate', ''),
                        transaction_date=getattr(trans, 'transactionDate', ''),
                        account_no=getattr(trans, 'accountNo', ''),
                        credit_amount=getattr(trans, 'creditAmount', ''),
                        debit_amount=getattr(trans, 'debitAmount', ''),
                        currency=getattr(trans, 'currency', 'VND'),
                        description=getattr(trans, 'description', ''),
                        add_description=getattr(trans, 'addDescription', ''),
                        available_balance=getattr(trans, 'availableBalance', ''),
                        ref_no=getattr(trans, 'refNo', ''),
                        ben_account_name=getattr(trans, 'benAccountName', ''),
                        bank_name=getattr(trans, 'bankName', ''),
                        transaction_type=getattr(trans, 'transactionType', ''),
                    ))
                    count += 1

            return transaction_pb2.GetTransactionsResponse(
                success=True,
                count=count,
                transactions=result_transactions
            )

        except Exception as e:
            logger.error(f"Error getting transactions: {str(e)}")
            return transaction_pb2.GetTransactionsResponse(
                success=False,
                count=0,
                transactions=[]
            )

    def Health(self, request, context):
        """Health check endpoint"""
        return transaction_pb2.HealthResponse(
            healthy=True,
            message="Transaction service is running"
        )


def serve(port=50051, max_workers=10, token=""):
    """Start gRPC server. Token rỗng chỉ được phép khi ALLOW_INSECURE_TRANSACTIONS=1 (dev), còn lại từ chối khởi động."""
    if not token:
        # load_service_token raise MissingServiceTokenError khi không có cờ dev; có cờ thì trả "" kèm cảnh báo.
        token = service_token.load_service_token({**os.environ, "TRANSACTION_SERVICE_TOKEN": ""})
    interceptors = [TokenAuthInterceptor(token)] if token else []
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=max_workers), interceptors=interceptors)
    transaction_pb2_grpc.add_TransactionServiceServicer_to_server(
        TransactionServicer(), server
    )

    server.add_insecure_port(f'[::]:{port}')
    server.start()

    logger.info(f"gRPC server started on port {port}")
    return server


if __name__ == '__main__':
    USERNAME = os.getenv("MB_USERNAME", "")
    PASSWORD = os.getenv("MB_PASSWORD", "")
    ACCOUNT_NO = os.getenv("MB_ACCOUNT_NO", "")

    server = serve(token=service_token.load_service_token())
    try:
        while True:
            time.sleep(86400)
    except KeyboardInterrupt:
        server.stop(0)
