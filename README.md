# 40Study Backend

Golang microservices cho nền tảng 40Study.

## Structure

```
backend/
├── gateway/              # API Gateway (port 8080)
│   ├── cmd/
│   └── internal/
│
├── services/             # Business microservices
│   ├── user/            # User & Auth (port 8081)
│   ├── course/          # Courses & Lessons (port 8082)
│   ├── media/           # Video & MinIO (port 8083)
│   └── ai/              # AI - Qwen 3B (port 8084)
│
├── shared/               # Common packages
│   └── pkg/
│       ├── config/
│       ├── database/
│       ├── middleware/
│       ├── errors/
│       └── utils/
│
└── go.work              # Go workspace
```

## Services

| Service | Port | Description |
|---------|------|-------------|
| gateway | 8080 | API Gateway - routing, auth, rate limit |
| user | 8081 | User & Authentication |
| course | 8082 | Courses & Lessons |
| media | 8083 | Video & File storage (MinIO) |
| ai | 8084 | AI - Transcription, Quiz (Qwen 3B) |

## Getting Started

```bash
# Run single service
cd services/user
go run cmd/main.go

# Run all with Docker
cd ../infra/docker
docker-compose up -d
```
