# MBBank Transaction API

API FastAPI để kiểm tra lịch sử giao dịch MB Bank và xác minh giao dịch qua mã PIN.

## Chạy nhanh với Docker Compose

Service đã được cấu hình sẵn trong `docker-compose.yaml`. Chỉ cần:

**1. Tạo file `.env` ở thư mục gốc:**
```env
MB_USERNAME=your_mbbank_username
MB_PASSWORD=your_mbbank_password
MB_ACCOUNT_NO=your_account_number
# BẮT BUỘC. Secret dùng chung với backend Go (cùng biến TRANSACTION_SERVICE_TOKEN trong .env của backend).
# Thiếu biến này service (cả REST lẫn gRPC) từ chối khởi động, docker-compose cũng không lên.
TRANSACTION_SERVICE_TOKEN=chuoi-ngau-nhien-dai
# Chỉ máy dev, thay cho token: ALLOW_INSECURE_TRANSACTIONS=1 (chạy không xác thực, có cảnh báo). KHÔNG dùng ở production.
```

**2. Chạy:**
```bash
docker-compose up -d
```

API sẽ chạy tại `http://localhost:8000`

**3. Xem logs:**
```bash
docker-compose logs -f mbbank-api
```

## Xác thực

Mọi endpoint REST bắt buộc header `X-Transaction-Token: <TRANSACTION_SERVICE_TOKEN>`; thiếu hoặc sai trả 401, và khi
service chưa cấu hình token thì trả 503 (đóng cửa, không mở). gRPC (`:50051`) cũng đóng cửa: đọc cùng token ở metadata
`x-transaction-token`, thiếu hoặc sai trả `UNAUTHENTICATED`; chưa đặt `TRANSACTION_SERVICE_TOKEN` thì server từ chối
khởi động (trừ khi `ALLOW_INSECURE_TRANSACTIONS=1` ở máy dev, khi đó có cảnh báo). Backend Go phải gửi cùng token trước
khi bật service này, nếu không luồng thanh toán bị từ chối.

## Một container, hai cổng

Container chạy cả REST (`:8000`) và gRPC (`:50051`, backend Go gọi ở đây) qua `run_services.py`. Một trong hai
tiến trình dừng thì cả container thoát mã khác 0 để `restart: unless-stopped` dựng lại. Mã gRPC (`transaction_pb2*.py`)
được sinh từ `transaction.proto` lúc build image, không nằm trong repo. Healthcheck (`healthcheck.py`) kiểm `GET /health`
(không cần token, chỉ trả `{"status": "ok"}`) và cổng gRPC.

## gRPC `CheckTransaction` (luồng thanh toán của backend Go)

Backend Go gọi RPC này (`:50051`) để tìm tiền khách đã chuyển cho một mã đơn (`payment_code`) trong khoảng
`from_timestamp`..`to_timestamp`. Khách có thể chuyển **nhiều lần** vào cùng một mã, nên phản hồi mang MỌI giao
dịch khớp, không chỉ giao dịch đầu của sao kê:

- `transactions` (field 9, lặp `MatchedTransaction`): mọi giao dịch khớp, theo thứ tự sao kê. Mỗi phần tử có
  `transaction_id` (`refNo` của MB), `amount` (`creditAmount`), `currency`, `description`, `transaction_date`.
- `found`, `transaction_id`, `amount`, `currency`, `description`, `transaction_date` (field 1-6): giữ nguyên nghĩa
  cũ, luôn là giao dịch khớp **đầu tiên**, để client cũ chưa biết `transactions` vẫn chạy đúng. Backend Go mới đọc
  `transactions` và tự dựng một phần tử từ các field này khi gặp service cũ không gửi danh sách.
- `status`: `success` (có giao dịch khớp), `not_found`, hoặc `error` (kèm `error_message`).

Một giao dịch **khớp** khi `payment_code` nằm trong `description` hoặc `addDescription` **và** là giao dịch ghi có
(`creditAmount` > 0). Giao dịch ghi nợ bị bỏ qua dù có chứa mã: khoản hoàn tiền của admin cũng ghi mã đơn trong
nội dung, nếu coi là tiền khách chuyển thì backend sẽ gắn cờ "cần hoàn tiền" cho chính khoản hoàn đó. Số tiền
không đọc được thì giữ giao dịch lại để backend tự đối chiếu, không giấu một khoản có thể là tiền thật.

Backend Go xử lý từng giao dịch: dedupe theo `transaction_id`, nên poll lại cùng sao kê không ghi trùng, còn lần
chuyển thứ hai vào một đơn đã đóng được gắn cờ hoàn tiền muộn như lần đầu.

Mã gRPC (`transaction_pb2*.py`) vẫn sinh từ `transaction.proto` lúc build; phía Go có bản sao ở
`internal/grpc/transaction.proto` và các struct viết tay trong `internal/grpc/transaction_grpc.pb.go`; đổi proto
thì sửa cả hai nơi.

## API Endpoints

- `GET /transactions` - Lấy danh sách giao dịch
- `GET /transactions/count` - Đếm số giao dịch
- `GET /transactions/check-pin` - Kiểm tra giao dịch theo PIN (15 ký tự); trả MỌI giao dịch khớp (`match_count`,
  `transactions`), không lọc ghi có/ghi nợ. Backend Go không gọi endpoint REST này, luồng thanh toán dùng gRPC ở trên.

**Query Parameters:**
- `from_date`, `to_date`: Format `hh-mm-ss-dd-mm-yyyy` (VD: `00-00-00-01-01-2024`)
- `pin`: Mã PIN 15 ký tự

**API Docs:** `http://localhost:8000/docs`