package app

import (
	"log"

	"github.com/minio/minio-go/v7"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"study.com/v1/internal/cache"
	"study.com/v1/internal/config"
	"study.com/v1/internal/database"
	"study.com/v1/internal/storage"
)

type Resources struct {
	DB          *gorm.DB
	Redis       *redis.Client
	MinioClient *minio.Client
	Config      *config.Config
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

	return &Resources{
		DB:          db,
		Redis:       rdb,
		MinioClient: minioClient,
		Config:      cfg,
	}, nil
}

func (r *Resources) Close() error {
	if r.Redis != nil {
		if err := r.Redis.Close(); err != nil {
			log.Printf("Error closing Redis: %v", err)
			return err
		}
	}
	return nil
}
