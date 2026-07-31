# Chạy local & seed dữ liệu demo

Hướng dẫn dựng đủ backend + dữ liệu demo để web có nội dung hiển thị.

## 1. Hạ tầng

```bash
cd backend
docker compose up -d postgres redis minio minio_createbuckets rabbitmq
```

Bốn service này là đủ cho luồng duyệt khoá học / đăng nhập. Các service còn lại
trong `docker-compose.yaml` (judge0, livekit, nginx, mbbank-api) chỉ cần khi làm
việc với chấm bài, livestream hoặc thanh toán.

## 2. Cấu hình `.env`

Các biến host phải trỏ về máy local:

```
PORT=5000
DB_HOST=localhost
REDIS_HOST=localhost
MINIO_HOST=localhost

RABBITMQ_HOST=localhost
RABBITMQ_PORT=5672
RABBITMQ_USER=guest
RABBITMQ_PASSWORD=guest
RABBITMQ_VHOST=/
```

Thiếu nhóm `RABBITMQ_*` thì backend vẫn chạy nhưng báo `403 username or password
not allowed`, kèm theo đó là queue bài tập / chứng chỉ / lời mời phụ huynh không
hoạt động.

## 3. Seed dữ liệu

```bash
go run ./cmd/seed              # roles/permissions + toàn bộ dữ liệu demo
go run ./cmd/seed -mode=base   # chỉ roles/permissions
```

Command từ chối chạy khi `ENVIRONMENT=prod` để tránh nạp tài khoản/mật khẩu demo
vào database thật. Trường hợp đặc biệt bắt buộc phải seed production cần truyền
`-allow-production` một cách tường minh.

Seeder idempotent — chạy lại nhiều lần không sinh bản ghi trùng (tra theo email,
slug, code trước khi tạo).

Dữ liệu sinh ra: 6 user, 2 tổ chức, 6 danh mục, 21 tag, 5 khoá học kèm chương +
bài học + nội dung bài học, 4 lượt ghi danh kèm tiến độ, 3 voucher.

## 4. Chạy server

```bash
go run ./cmd/api    # backend :5000, tự AutoMigrate khi khởi động
```

## 5. Tài khoản demo

Mật khẩu chung: `Demo@123`

| Email | Vai trò |
|-------|---------|
| admin@demo.com | SYSTEM_ADMIN |
| teacher1@demo.com | TEACHER (Nguyễn Văn A) |
| teacher2@demo.com | TEACHER (Trần Thị B) |
| student1@demo.com | STUDENT (Lê Văn C) — đã ghi danh 3 khoá |
| student2@demo.com | STUDENT (Phạm Thị D) — đã ghi danh 1 khoá |
| parent1@demo.com | PARENT (Hoàng Văn E) |

Endpoint login yêu cầu `device_info.device_id` là UUID hợp lệ:

```json
{
  "email": "student1@demo.com",
  "password": "Demo@123",
  "device_info": { "device_id": "<uuid>", "device_name": "Chrome", "os": "windows" }
}
```

Login có rate limit — gọi liên tiếp nhiều tài khoản sẽ nhận `429`, chờ rồi thử lại.

## 6. Web

Trong `web/.env`:

```
NEXT_PUBLIC_API_URL=http://localhost:5000/api
```

```bash
cd web && npm install && npm run dev   # :3000
```

Backend đã bật CORS cho `http://localhost:3000` kèm credentials.
