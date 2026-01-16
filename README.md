# 40Study Backend

Golang microservices cho nền tảng 40Study.

## Services

| Service | Port | Description |
|---------|------|-------------|
| gateway | 8080 | API Gateway - routing, auth, rate limit |
| user-service | 8081 | User & Authentication |
| course-service | 8082 | Courses & Lessons |
| media-service | 8083 | Video & File storage (MinIO) |
| ai-service | 8084 | AI - Transcription, Quiz (Qwen 3B) |

## Structure

```
backend/
├── gateway/
├── user-service/
├── course-service/
├── media-service/
├── ai-service/
└── shared/            # Common packages
    └── pkg/
        ├── config/
        ├── database/
        ├── middleware/
        ├── errors/
        └── utils/
```

## Getting Started

```bash
# Run single service
cd user-service
go run cmd/main.go

# Run all with Docker
cd ../infra/docker
docker-compose up -d
```
