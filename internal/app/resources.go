package app

import (
	"context"
	"log"

	"github.com/minio/minio-go/v7"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"study.com/v1/internal/cache"
	"study.com/v1/internal/config"
	"study.com/v1/internal/database"
	asynq_queue "study.com/v1/internal/queue/asynq"
	rabbitmq_queue "study.com/v1/internal/queue/rabbitmq"
	"study.com/v1/internal/storage"
)

type Resources struct {
	DB           *gorm.DB
	Redis        *redis.Client
	MinioClient  *minio.Client
	MinioWrapper *storage.MinioClient
	RabbitMQ     *rabbitmq_queue.RabbitMQService
	Config       *config.Config
	Queue        *asynq_queue.Queue
}

func InitResources() (*Resources, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
		return nil, err
	}

	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
		return nil, err
	}

	if err := database.Migrate(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
		return nil, err
	}

	rdb, err := cache.Connect(cfg)
	if err != nil {
		log.Printf("Warning: Failed to connect to redis: %v", err)
	}

	minioClient, err := storage.Connect(cfg)
	if err != nil {
		log.Printf("Warning: Failed to connect to minio: %v", err)
	}

	minioWrapper, err := storage.NewMinioClient(cfg)
	if err != nil {
		log.Printf("Warning: Failed to create minio wrapper client: %v", err)
	} else if minioWrapper != nil {
		if err := minioWrapper.EnsureBuckets(context.Background()); err != nil {
			log.Printf("Warning: Failed to ensure MinIO buckets: %v", err)
		}
	}

	rabbitMQ, err := rabbitmq_queue.NewRabbitMQService(cfg)
	if err != nil {
		log.Printf("Warning: Failed to connect to RabbitMQ: %v", err)
	}

	aq := asynq_queue.New(cfg)
	return &Resources{
		DB:           db,
		Redis:        rdb,
		MinioClient:  minioClient,
		MinioWrapper: minioWrapper,
		RabbitMQ:     rabbitMQ,
		Config:       cfg,
		Queue:        aq,
	}, nil
}

func (r *Resources) Close() error {
	if r.Redis != nil {
		if err := r.Redis.Close(); err != nil {
			log.Printf("Error closing Redis: %v", err)
			return err
		}
	}
	if r.RabbitMQ != nil {
		if err := r.RabbitMQ.Close(); err != nil {
			log.Printf("Error closing RabbitMQ: %v", err)
		}
	}
	if r.Queue != nil {
		r.Queue.Stop()
	}
	return nil
}
