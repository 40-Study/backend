"""Dùng chung cho pytest của service giao dịch: thay mbbank bằng bản giả TRƯỚC khi import api / grpc_server,
để test không cần MB Bank thật (cũng không cần cài mbbank-lib)."""
import sys
import types


class FakeMBBank:
    def __init__(self, username="", password=""):
        pass

    def getTransactionAccountHistory(self, accountNo, from_date, to_date):
        return types.SimpleNamespace(transactionHistoryList=[])


sys.modules.setdefault("mbbank", types.SimpleNamespace(MBBank=FakeMBBank))
