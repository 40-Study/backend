package config

import (
	"flag"
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Environment string `mapstructure:"ENVIRONMENT"`
	Port        string `mapstructure:"PORT"`
	Host        string `mapstructure:"HOST"`

	DBHost     string `mapstructure:"DB_HOST"`
	DBPort     string `mapstructure:"DB_PORT"`
	DBUser     string `mapstructure:"DB_USER"`
	DBPassword string `mapstructure:"DB_PASSWORD"`
	DBName     string `mapstructure:"DB_NAME"`

	RedisHost     string `mapstructure:"REDIS_HOST"`
	RedisPort     string `mapstructure:"REDIS_PORT"`
	RedisPassword string `mapstructure:"REDIS_PASSWORD"`
	RedisDB       int    `mapstructure:"REDIS_DB"`

	MinioHost      string `mapstructure:"MINIO_HOST"`
	MinioPort      string `mapstructure:"MINIO_PORT"`
	MinioAccessKey string `mapstructure:"MINIO_ACCESS_KEY"`
	MinioSecretKey string `mapstructure:"MINIO_SECRET_KEY"`
	MinioUseSSL    bool   `mapstructure:"MINIO_USE_SSL"`

	// Minio Buckets
	MinioBucketImages string `mapstructure:"MINIO_BUCKET_IMAGES"`
	MinioBucketVideos string `mapstructure:"MINIO_BUCKET_VIDEOS"`

	// SMTP Configuration
	SMTPHost     string `mapstructure:"SMTP_HOST"`
	SMTPPort     int    `mapstructure:"SMTP_PORT"`
	SMTPUser     string `mapstructure:"SMTP_USERNAME"`
	SMTPPassword string `mapstructure:"SMTP_PASSWORD"`
	SMTPFrom     string `mapstructure:"FROM_EMAIL"`

	// JWT Configuration
	JWTSecret            string `mapstructure:"JWT_SECRET"`
	JWTAccessExpiration  time.Duration
	JWTRefreshExpiration time.Duration
}

func LoadConfig() (*Config, error) {
	var env string
	flag.StringVar(&env, "env", "dev", "Environment (dev, test, prod)")
	flag.Parse()

	config := &Config{}
	viper.Set("ENVIRONMENT", env)

	// Set default values
	viper.SetDefault("PORT", "3000")
	viper.SetDefault("HOST", "localhost")
	viper.SetDefault("REDIS_DB", 0)
	viper.SetDefault("MINIO_USE_SSL", false)
	viper.SetDefault("MINIO_BUCKET_IMAGES", "images")
	viper.SetDefault("MINIO_BUCKET_VIDEOS", "videos")

	viper.AutomaticEnv()

	// If running in dev or test, try to load .env file
	if env == "dev" || env == "test" {
		viper.SetConfigName(".env")
		viper.SetConfigType("env")
		viper.AddConfigPath(".")
		viper.AddConfigPath("../")
		viper.AddConfigPath("../../")

		if err := viper.ReadInConfig(); err != nil {
			// It's okay if config file doesn't exist, we might rely on env vars
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				return nil, fmt.Errorf("error reading config file: %w", err)
			}
			fmt.Println("No .env file found, relying on environment variables")
		} else {
			fmt.Println("Loaded configuration from .env file")
		}
	}
	if env == "test" || env == "dev" {
		// If standard DB_HOST is not set, try TEST_DB_HOST
		if viper.GetString("DB_HOST") == "" && viper.GetString("TEST_DB_HOST") != "" {
			viper.Set("DB_HOST", viper.GetString("TEST_DB_HOST"))
		}
		if viper.GetString("DB_PORT") == "" && viper.GetString("TEST_DB_PORT") != "" {
			viper.Set("DB_PORT", viper.GetString("TEST_DB_PORT"))
		}
		if viper.GetString("DB_USER") == "" && viper.GetString("TEST_DB_USER") != "" {
			viper.Set("DB_USER", viper.GetString("TEST_DB_USER"))
		}
		if viper.GetString("DB_PASSWORD") == "" && viper.GetString("TEST_DB_PASSWORD") != "" {
			viper.Set("DB_PASSWORD", viper.GetString("TEST_DB_PASSWORD"))
		}
		if viper.GetString("DB_NAME") == "" && viper.GetString("TEST_DB_NAME") != "" {
			viper.Set("DB_NAME", viper.GetString("TEST_DB_NAME"))
		}
		if viper.GetString("PORT") == "3000" && viper.GetString("TEST_PORT") != "" { // Default is 3000
			viper.Set("PORT", viper.GetString("TEST_PORT"))
		}
		if viper.GetString("HOST") == "localhost" && viper.GetString("TEST_HOST") != "" {
			viper.Set("HOST", viper.GetString("TEST_HOST"))
		}
	}

	// Set defaults for SMTP and JWT
	viper.SetDefault("SMTP_HOST", "smtp.gmail.com")
	viper.SetDefault("SMTP_PORT", 587)
	viper.SetDefault("JWT_SECRET", "supersecretkey-change-in-production")
	viper.SetDefault("JWT_ACCESS_EXPIRATION_MINUTES", 15)
	viper.SetDefault("JWT_REFRESH_EXPIRATION_DAYS", 7)

	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("unable to decode into struct: %w", err)
	}

	// Set JWT expiration durations
	accessMinutes := viper.GetInt("JWT_ACCESS_EXPIRATION_MINUTES")
	refreshDays := viper.GetInt("JWT_REFRESH_EXPIRATION_DAYS")
	config.JWTAccessExpiration = time.Duration(accessMinutes) * time.Minute
	config.JWTRefreshExpiration = time.Duration(refreshDays) * 24 * time.Hour

	return config, nil
}
