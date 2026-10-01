"""L1: CheckTransaction trả MỌI giao dịch ghi có khớp mã đơn, không chỉ giao dịch đầu của sao kê.

Khách chuyển khoản nhiều lần vào cùng một mã đơn; trước đây chỉ giao dịch khớp đầu tiên tới được backend Go
nên lần chuyển thứ hai không bao giờ được thấy và luồng cờ hoàn tiền muộn không bật.

Test dùng mã sinh THẬT từ transaction.proto (grpc_tools.protoc, giống bước build của dockerfile) và đi qua
một server gRPC thật trên cổng tạm, để field/số thứ tự sai trong proto làm test đỏ.

Chạy: cd transaction && python -m pytest -q
"""
import importlib.util
import os
import shutil
import socket
import subprocess
import sys
import types

import grpc
import pytest

HERE = os.path.dirname(os.path.abspath(__file__))
TOKEN = "l1-shared-secret"
CODE = "40STUDY ORD20261002ABCDEF12"


def _tx(ref, credit, desc=None, add_desc="", debit="0", date="02/10/2026 09:00:00"):
    return types.SimpleNamespace(
        refNo=ref, creditAmount=credit, debitAmount=debit,
        description=desc if desc is not None else f"MBVCB {CODE} chuyen tien",
        addDescription=add_desc, transactionDate=date,
    )


@pytest.fixture(scope="module")
def pb(tmp_path_factory):
    """transaction_pb2 / transaction_pb2_grpc sinh từ proto thật, nạp vào sys.modules cho grpc_server."""
    out = tmp_path_factory.mktemp("pb")
    # Chép proto vào thư mục tạm và chạy protoc ở đó: protoc trên Windows không đọc được đường dẫn có
    # ký tự không phải ASCII (thư mục dự án tên "ĐỒ án"), còn đường dẫn tương đối thì luôn ổn.
    shutil.copy(os.path.join(HERE, "transaction.proto"), out / "transaction.proto")
    subprocess.run(
        [sys.executable, "-m", "grpc_tools.protoc", "-I.", "--python_out=.", "--grpc_python_out=.", "transaction.proto"],
        check=True, cwd=out,
    )
    mods = {}
    for name in ("transaction_pb2", "transaction_pb2_grpc"):
        spec = importlib.util.spec_from_file_location(name, out / f"{name}.py")
        mods[name] = importlib.util.module_from_spec(spec)
    saved = {n: sys.modules.get(n) for n in (*mods, "grpc_server")}
    sys.modules.update(mods)
    for name in mods:  # transaction_pb2_grpc import transaction_pb2 nên nạp theo thứ tự
        mods[name].__spec__.loader.exec_module(mods[name])
    sys.modules.pop("grpc_server", None)
    yield mods["transaction_pb2"], mods["transaction_pb2_grpc"]
    for name, mod in saved.items():
        if mod is None:
            sys.modules.pop(name, None)
        else:
            sys.modules[name] = mod


def _free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


@pytest.fixture()
def call(pb, monkeypatch):
    """Gọi CheckTransaction qua server gRPC thật; `history` là sao kê MB giả."""
    pb2, pb2_grpc = pb
    import grpc_server

    servers = []

    def run(history):
        monkeypatch.setattr(
            grpc_server, "get_mbbank_client",
            lambda: types.SimpleNamespace(
                getTransactionAccountHistory=lambda **_: types.SimpleNamespace(transactionHistoryList=history)
            ),
        )
        port = _free_port()
        server = grpc_server.serve(port=port, token=TOKEN)
        servers.append(server)
        with grpc.insecure_channel(f"127.0.0.1:{port}") as ch:
            stub = pb2_grpc.TransactionServiceStub(ch)
            return stub.CheckTransaction(
                pb2.CheckTransactionRequest(payment_code=CODE, from_timestamp=1, to_timestamp=2_000_000_000),
                metadata=(("x-transaction-token", TOKEN),), timeout=10,
            )

    yield run
    for s in servers:
        s.stop(0)


def test_returns_every_matching_credit_in_statement_order(call):
    resp = call([
        _tx("REF-1", "100000"),
        _tx("REF-OTHER", "999", desc="chuyen tien khac, khong lien quan"),
        _tx("REF-2", "250000", date="03/10/2026 10:30:00"),
    ])
    assert resp.found and resp.status == "success"
    assert [t.transaction_id for t in resp.transactions] == ["REF-1", "REF-2"]
    assert [t.amount for t in resp.transactions] == ["100000", "250000"]
    assert resp.transactions[1].transaction_date == "03/10/2026 10:30:00"
    assert all(t.currency == "VND" for t in resp.transactions)


def test_legacy_fields_still_describe_the_first_match(call):
    """Tương thích ngược: client cũ chỉ đọc field 2-6 phải nhận đúng giao dịch khớp đầu tiên."""
    resp = call([_tx("REF-1", "100000"), _tx("REF-2", "250000")])
    assert (resp.transaction_id, resp.amount, resp.currency) == ("REF-1", "100000", "VND")
    assert CODE in resp.description
    assert resp.transaction_date == "02/10/2026 09:00:00"


def test_code_in_add_description_also_matches(call):
    resp = call([_tx("REF-ADD", "70000", desc="chuyen tien", add_desc=f"ghi chu {CODE}")])
    assert [t.transaction_id for t in resp.transactions] == ["REF-ADD"]


def test_debit_rows_mentioning_the_code_are_not_incoming_money(call):
    """Khoản hoàn tiền của admin cũng ghi mã đơn nhưng là ghi nợ: không được coi là khách chuyển."""
    resp = call([
        _tx("REF-REFUND", "0", debit="100000", desc=f"HOAN TIEN {CODE}"),
        _tx("REF-IN", "100000"),
    ])
    assert [t.transaction_id for t in resp.transactions] == ["REF-IN"]
    assert resp.transaction_id == "REF-IN"  # kể cả field cũ: không lấy khoản hoàn làm "giao dịch đầu"


def test_only_debit_match_is_not_found(call):
    resp = call([_tx("REF-REFUND", "0", debit="100000", desc=f"HOAN TIEN {CODE}")])
    assert not resp.found and resp.status == "not_found" and list(resp.transactions) == []


def test_not_found_has_empty_list(call):
    resp = call([_tx("REF-X", "5", desc="khong lien quan")])
    assert not resp.found and resp.status == "not_found" and list(resp.transactions) == []


def test_unparseable_credit_is_kept_not_hidden(call):
    """Không đọc được số tiền thì GIỮ lại cho Go đối chiếu, không giấu một khoản có thể là tiền thật."""
    resp = call([_tx("REF-ODD", "1.2.3")])
    assert [t.transaction_id for t in resp.transactions] == ["REF-ODD"]


def test_thousands_separator_credit_counts_as_credit(call):
    resp = call([_tx("REF-COMMA", "1,500,000")])
    assert [t.transaction_id for t in resp.transactions] == ["REF-COMMA"]
