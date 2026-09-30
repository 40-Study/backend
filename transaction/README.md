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
service chưa cấu hình token thì trả 503 (đóng cửa, không mở). gRPC (`:50051`) đọc cùng token ở metadata
`x-transaction-token` khi biến được đặt; chưa đặt thì server ghi cảnh báo và nhận mọi request (giữ luồng thanh toán
đang chạy). Bật theo thứ tự: backend Go trước, service này sau.

## Một container, hai cổng

Container chạy cả REST (`:8000`) và gRPC (`:50051`, backend Go gọi ở đây) qua `run_services.py`. Một trong hai
tiến trình dừng thì cả container thoát mã khác 0 để `restart: unless-stopped` dựng lại. Mã gRPC (`transaction_pb2*.py`)
được sinh từ `transaction.proto` lúc build image, không nằm trong repo. Healthcheck (`healthcheck.py`) kiểm `GET /health`
(không cần token, chỉ trả `{"status": "ok"}`) và cổng gRPC.

## API Endpoints

- `GET /transactions` - Lấy danh sách giao dịch
- `GET /transactions/count` - Đếm số giao dịch
- `GET /transactions/check-pin` - Kiểm tra giao dịch theo PIN (15 ký tự)

**Query Parameters:**
- `from_date`, `to_date`: Format `hh-mm-ss-dd-mm-yyyy` (VD: `00-00-00-01-01-2024`)
- `pin`: Mã PIN 15 ký tự

**API Docs:** `http://localhost:8000/docs`