# MBBank Transaction API

API FastAPI để kiểm tra lịch sử giao dịch MB Bank và xác minh giao dịch qua mã PIN.

## Chạy nhanh với Docker Compose

Service đã được cấu hình sẵn trong `docker-compose.yaml`. Chỉ cần:

**1. Tạo file `.env` ở thư mục gốc:**
```env
MB_USERNAME=your_mbbank_username
MB_PASSWORD=your_mbbank_password
MB_ACCOUNT_NO=your_account_number
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

## API Endpoints

- `GET /transactions` - Lấy danh sách giao dịch
- `GET /transactions/count` - Đếm số giao dịch
- `GET /transactions/check-pin` - Kiểm tra giao dịch theo PIN (15 ký tự)

**Query Parameters:**
- `from_date`, `to_date`: Format `hh-mm-ss-dd-mm-yyyy` (VD: `00-00-00-01-01-2024`)
- `pin`: Mã PIN 15 ký tự

**API Docs:** `http://localhost:8000/docs`