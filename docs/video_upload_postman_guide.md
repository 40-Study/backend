# Video Upload API – Postman Testing Guide

Base URL: `http://localhost:3000`

---

## 0. Đăng ký tài khoản (2 bước)

### Bước 1 – POST /api/auth/register/request

Gửi thông tin đăng ký → hệ thống gửi OTP về email.

```
POST {{base_url}}/api/auth/register/request
Content-Type: application/json
```

**Body:**
```json
{
  "email": "student@example.com",
  "password": "SecurePass123!",
  "confirm_password": "SecurePass123!",
  "user_name": "student123",
  "full_name": "Nguyen Van A",
  "role_ids": ["550e8400-e29b-41d4-a716-446655440000"]
}
```

> `role_ids`: mảng UUID của role, ít nhất 1, tối đa 2.  
> `password`: tối thiểu 8 ký tự.  
> `confirm_password`: phải khớp với `password`.

**Expected response (200):**
```json
{
  "id": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "email": "student@example.com",
  "user_name": "student123",
  "full_name": "Nguyen Van A",
  "roles": [
    { "id": "...", "code": "student", "name": "Học sinh" }
  ]
}
```

---

### Bước 2 – POST /api/auth/register (xác thực OTP)

```
POST {{base_url}}/api/auth/register
Content-Type: application/json
```

**Body:**
```json
{
  "email": "student@example.com",
  "otp": "123456"
}
```

> OTP gồm 6 chữ số, nhận qua email sau bước 1.

**Expected response (200):**
```json
{
  "user": {
    "id": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
    "username": "student123",
    "email": "student@example.com",
    "is_active": true,
    "created_at": "2026-03-01T00:00:00Z"
  }
}
```

---

## Chuẩn bị: Lấy JWT Token

### POST /api/auth/login

```
POST {{base_url}}/api/auth/login
Content-Type: application/json
```

**Body:**
```json
{
  "email": "your@email.com",
  "password": "yourpassword",
  "device_info": {
    "device_id": "550e8400-e29b-41d4-a716-446655440000",
    "device_name": "Postman Test",
    "os": "Windows 11",
    "app_version": "1.0.0",
    "user_agent": "PostmanRuntime/7.36.0"
  }
}
```
**Lưu token vào Postman variable:**  
Trong tab **Tests** của request này:
```js
const res = pm.response.json();
pm.environment.set("token", res.data.access_token);
pm.environment.set("user_id", res.data.user.id);
```

> Tất cả request dưới đây cần header:  
> `Authorization: Bearer {{token}}`

---

## 1. Health Check (không cần auth)

### GET /api/videos/health

```
GET {{base_url}}/api/videos/health
```

**Expected response (200):**
```json
{
  "message": "Video upload service is running",
  "status": "ok"
}
```

---

## 2. Init Upload Session

### POST /api/videos/upload/init

```
POST {{base_url}}/api/videos/upload/init
Authorization: Bearer {{token}}
Content-Type: application/json
```

**Body:**
```json
{
  "resource_id": "00000000-0000-0000-0000-000000000001",
  "resource_type": "video",
  "original_file_name": "demo.mp4",
  "content_type": "video/mp4",
  "file_size": 52428800,
  "chunk_size": 10485760
}
```

> `file_size`: 50 MB = 52,428,800 bytes  
> `chunk_size`: 10 MB = 10,485,760 bytes (optional, default 10MB)  
> `resource_type`: `video` | `course` | `blog` | `user` | `product`

**Expected response (200):**
```json
{
  "upload_id": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "upload_key": "minio-multipart-upload-id",
  "object_key": "videos/2026/03/01/uuid.mp4",
  "chunk_size": 10485760,
  "total_chunks": 5,
  "expires_at": 1740000000
}
```

**Lưu vào biến:**
```js
const res = pm.response.json();
pm.environment.set("upload_id", res.upload_id);
pm.environment.set("total_chunks", res.total_chunks);
```

---

## 3. Lấy Presigned URLs

### POST /api/videos/upload/presigned-urls

```
POST {{base_url}}/api/videos/upload/presigned-urls
Authorization: Bearer {{token}}
Content-Type: application/json
```

**Body (lấy URLs cho tất cả 5 chunks):**
```json
{
  "upload_id": "{{upload_id}}",
  "chunk_numbers": [1, 2, 3, 4, 5]
}
```

**Body (lấy từng chunk khi resume):**
```json
{
  "upload_id": "{{upload_id}}",
  "chunk_numbers": [3, 4, 5]
}
```

**Expected response (200):**
```json
{
  "upload_id": "{{upload_id}}",
  "urls": [
    {
      "chunk_number": 1,
      "url": "http://minio:9000/videos/...?X-Amz-Signature=...",
      "expires_at": 1740001800
    },
    ...
  ]
}
```

> Dùng các URL này để upload trực tiếp lên MinIO bằng **PUT** request.  
> Lưu ETag từ response header của mỗi PUT để dùng ở bước 4.

---

## 4. Upload Chunk trực tiếp lên MinIO (PUT)

> Đây là request PUT tới presigned URL đã lấy ở bước 3, **không qua backend**.

```
PUT <presigned_url_from_step_3>
Content-Type: video/mp4
Body: [binary chunk data]
```

**Lưu ETag từ response header:**
```js
// Trong Tests tab
const etag = pm.response.headers.get("ETag");
pm.environment.set("chunk_1_etag", etag);
```

---

## 5. Thông báo chunk đã upload xong

### POST /api/videos/upload/chunk-complete

```
POST {{base_url}}/api/videos/upload/chunk-complete
Authorization: Bearer {{token}}
Content-Type: application/json
```

**Body (lặp cho từng chunk):**
```json
{
  "upload_id": "{{upload_id}}",
  "chunk_number": 1,
  "etag": "\"abc123def456\"",
  "size": 10485760
}
```

**Expected response (200):**
```json
{
  "success": true,
  "chunk_number": 1,
  "progress": 20.0,
  "uploaded_chunks": 1,
  "total_chunks": 5,
  "is_completed": false
}
```

> Khi chunk cuối cùng được báo: `"is_completed": true`

---

## 6. Hoàn tất Upload (trigger processing)

### POST /api/videos/upload/complete

```
POST {{base_url}}/api/videos/upload/complete
Authorization: Bearer {{token}}
Content-Type: application/json
```

**Body:**
```json
{
  "upload_id": "{{upload_id}}"
}
```

**Expected response (200):**
```json
{
  "success": true,
  "upload_id": "{{upload_id}}",
  "status": "processing",
  "object_key": "videos/2026/03/01/uuid.mp4",
  "message": "Upload completed. Video processing started."
}
```

---

## 7. Kiểm tra trạng thái Upload

### GET /api/videos/upload/:upload_id/status

```
GET {{base_url}}/api/videos/upload/{{upload_id}}/status
Authorization: Bearer {{token}}
```

**Expected response (200) – đang xử lý:**
```json
{
  "upload_id": "{{upload_id}}",
  "status": "processing",
  "progress": 100.0,
  "uploaded_chunks": 5,
  "total_chunks": 5,
  "file_size": 52428800,
  "processing_started_at": 1740000100,
  "retry_count": 0,
  "created_at": 1740000000,
  "updated_at": 1740000100
}
```

**Expected response (200) – đã xong:**
```json
{
  "upload_id": "{{upload_id}}",
  "status": "ready",
  "progress": 100.0,
  "thumbnail_url": "http://...",
  "duration": 125.5,
  "resolution": "1920x1080",
  "processing_completed_at": 1740000300
}
```

**Possible status values:**

| Status | Ý nghĩa |
|---|---|
| `initialized` | Session vừa tạo |
| `uploading` | Đang upload chunks |
| `uploaded` | Tất cả chunks xong |
| `processing` | FFmpeg đang xử lý |
| `ready` | Sẵn sàng xem |
| `failed` | Lỗi xử lý |
| `aborted` | Đã hủy |

---

## 8. Lấy thông tin Resume

### GET /api/videos/upload/:upload_id/resume

```
GET {{base_url}}/api/videos/upload/{{upload_id}}/resume
Authorization: Bearer {{token}}
```

**Expected response (200):**
```json
{
  "upload_id": "{{upload_id}}",
  "object_key": "videos/2026/03/01/uuid.mp4",
  "file_name": "demo.mp4",
  "file_size": 52428800,
  "can_resume": true,
  "completed_chunks": [1, 2],
  "pending_chunks": [3, 4, 5],
  "chunk_size": 10485760,
  "total_chunks": 5,
  "progress": 40.0
}
```

---

## 9. Danh sách Uploads chưa hoàn thành

### GET /api/videos/upload/incomplete

```
GET {{base_url}}/api/videos/upload/incomplete
Authorization: Bearer {{token}}
```

**Expected response (200):**
```json
[
  {
    "id": "{{upload_id}}",
    "status": "uploading",
    "original_file_name": "demo.mp4",
    "progress": 40.0,
    "created_at": "2026-03-01T10:00:00Z"
  }
]
```

---

## 10. Processing Queue

### GET /api/videos/processing/queue

```
GET {{base_url}}/api/videos/processing/queue
Authorization: Bearer {{token}}
```

**Expected response (200):**
```json
[
  {
    "id": "{{upload_id}}",
    "status": "processing",
    "original_file_name": "demo.mp4",
    "progress": 100.0
  }
]
```

---

## 11. Hủy Upload

### DELETE /api/videos/upload/:upload_id

```
DELETE {{base_url}}/api/videos/upload/{{upload_id}}
Authorization: Bearer {{token}}
Content-Type: application/json
```

**Body (optional):**
```json
{
  "upload_id": "{{upload_id}}",
  "reason": "User cancelled"
}
```

**Expected response (200):**
```json
{
  "success": true,
  "upload_id": "{{upload_id}}",
  "message": "Upload aborted successfully"
}
```

---

## 12. HLS Streaming (không cần auth)

### 12.1 Get Video Info

```
GET {{base_url}}/api/hls/{{upload_id}}/info
```

**Expected response (200):**
```json
{
  "upload_id": "{{upload_id}}",
  "duration": 125.5,
  "resolution": "1920x1080",
  "qualities": ["480p", "720p"],
  "master_url": "/api/hls/{{upload_id}}/master.m3u8"
}
```

### 12.2 Get Master Playlist

```
GET {{base_url}}/api/hls/{{upload_id}}/master.m3u8
```

> Response: `Content-Type: application/vnd.apple.mpegurl` (file `.m3u8`)

### 12.3 Get Quality Playlist

```
GET {{base_url}}/api/hls/{{upload_id}}/720p/index.m3u8
GET {{base_url}}/api/hls/{{upload_id}}/480p/index.m3u8
```

### 12.4 Get Segment

```
GET {{base_url}}/api/hls/{{upload_id}}/720p/seg_00001.ts
```

---

## Full Upload Flow (Order)

```
1. POST /api/auth/login                          → lấy token
2. POST /api/videos/upload/init                  → lấy upload_id, total_chunks
3. POST /api/videos/upload/presigned-urls        → lấy URLs cho tất cả chunks
4. PUT  <presigned_url> (x total_chunks)         → upload từng chunk lên MinIO, lấy ETag
5. POST /api/videos/upload/chunk-complete (x N)  → báo backend từng chunk xong
6. POST /api/videos/upload/complete              → hoàn tất, bắt đầu processing
7. GET  /api/videos/upload/:id/status            → poll cho đến khi status = "ready"
8. GET  /api/hls/:id/master.m3u8                 → stream video
```

---

## Postman Environment Variables

| Variable | Ví dụ | Ghi chú |
|---|---|---|
| `base_url` | `http://localhost:3000` | |
| `token` | `eyJhbG...` | Set tự động sau login |
| `upload_id` | UUID | Set tự động sau init |
| `total_chunks` | `5` | Set tự động sau init |
