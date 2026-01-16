package storage

import (
	"fmt"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"tiger.com/v2/src/config"
)

func Connect(cfg *config.Config) (*minio.Client, error) {
	endpoint := fmt.Sprintf("%s:%s", cfg.MinioHost, cfg.MinioPort)

	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinioAccessKey, cfg.MinioSecretKey, ""),
		Secure: cfg.MinioUseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create minio client: %w", err)
	}

	return minioClient, nil
}
