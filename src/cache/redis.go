package cache

import (
	"context"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
	"tiger.com/v2/src/config"
)

func Connect(cfg *config.Config) (*redis.Client, error) {
	addr := fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort)
	if cfg.RedisHost == "" {
		addr = "localhost:6379"
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: cfg.RedisPassword, // no password set
		DB:       cfg.RedisDB,       // use default DB
	})

	ctx := context.Background()
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	log.Println("Connected to Redis successfully")
	return rdb, nil
}
