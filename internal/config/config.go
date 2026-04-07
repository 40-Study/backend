package config

import (
	"flag"
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type GithubOAuthConfig struct {
	ClientID     string `mapstructure:"GITHUB_CLIENT_ID"`     // Github client ID lấy từ trang developer của Github
	ClientSecret string `mapstructure:"GITHUB_CLIENT_SECRET"` // Github client secret lấy từ trang developer của Github
	RedirectURL  string `mapstructure:"GITHUB_REDIRECT_URL"`  // URL mà Github sẽ redirect về sau khi user authorize, phải trùng với URL đã đăng ký trên trang developer của Github
	Endpoint     struct {
		AuthURL  string `mapstructure:"GITHUB_AUTH_URL"`  // URL của endpoint authorize của Github
		TokenURL string `mapstructure:"GITHUB_TOKEN_URL"` // URL của endpoint token của Github
	} `mapstructure:"GITHUB_ENDPOINT"`
	Scopes []string `mapstructure:"GITHUB_SCOPES"` // Các scope cần thiết để lấy thông tin user từ Github, ví dụ: "user:email" để lấy email của user
}

type GoogleOAuthConfig struct {
	ClientID     string `mapstructure:"GOOGLE_CLIENT_ID"`
	ClientSecret string `mapstructure:"GOOGLE_CLIENT_SECRET"`
	RedirectURL  string `mapstructure:"GOOGLE_REDIRECT_URL"`
}

type FacebookOAuthConfig struct {
	ClientID     string `mapstructure:"FACEBOOK_CLIENT_ID"`     // Facebook App ID
	ClientSecret string `mapstructure:"FACEBOOK_CLIENT_SECRET"` // Facebook App Secret
	RedirectURL  string `mapstructure:"FACEBOOK_REDIRECT_URL"`
}
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

	MinIOEndpoint   string `mapstructure:"MINIO_ENDPOINT"`    // host:port format
	MinIOBucketName string `mapstructure:"MINIO_BUCKET_NAME"` // Main bucket for videos

	// SMTP Configuration
	SMTPHost     string `mapstructure:"SMTP_HOST"`
	SMTPPort     int    `mapstructure:"SMTP_PORT"`
	SMTPUser     string `mapstructure:"SMTP_USERNAME"`
	SMTPPassword string `mapstructure:"SMTP_PASSWORD"`
	SMTPFrom     string `mapstructure:"FROM_EMAIL"`

	RabbitMQHost     string `mapstructure:"RABBITMQ_HOST"`
	RabbitMQPort     string `mapstructure:"RABBITMQ_PORT"`
	RabbitMQUser     string `mapstructure:"RABBITMQ_USER"`
	RabbitMQPassword string `mapstructure:"RABBITMQ_PASSWORD"`
	RabbitMQVHost    string `mapstructure:"RABBITMQ_VHOST"`

	// LiveKit Configuration
	LivekitNodeIP    string `mapstructure:"LIVEKIT_NODE_IP"`
	LivekitNodePort  string `mapstructure:"LIVEKIT_NODE_PORT"`
	LivekitAPIKey    string `mapstructure:"LIVEKIT_API_KEY"`
	LivekitAPISecret string `mapstructure:"LIVEKIT_API_SECRET"`
	LivekitURL       string `mapstructure:"LIVEKIT_URL"`

	// JWT Configuration
	JWTSecret            string `mapstructure:"JWT_SECRET"`
	JWTAccessExpiration  time.Duration
	JWTRefreshExpiration time.Duration

	// CORS
	AllowedOrigins string `mapstructure:"ALLOWED_ORIGINS"`

	// Transaction Service (MBBank gRPC)
	TransactionServiceHost string `mapstructure:"TRANSACTION_SERVICE_HOST"`
	TransactionServicePort string `mapstructure:"TRANSACTION_SERVICE_PORT"`

	// OAuth Providers
	GitHub   GithubOAuthConfig
	Google   GoogleOAuthConfig
	Facebook FacebookOAuthConfig

	// Frontend URL for OAuth redirect
	FrontendURL                string `mapstructure:"FRONTEND_URL"`
	ParentInvitationDailyLimit int    `mapstructure:"PARENT_INVITATION_DAILY_LIMIT"` // Số lần gửi lời mời phụ huynh tối đa trong 24 giờ của mỗi học sinh
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
	viper.SetDefault("MINIO_BUCKET_NAME", "videos")

	// Transaction Service
	viper.SetDefault("TRANSACTION_SERVICE_HOST", "localhost")
	viper.SetDefault("TRANSACTION_SERVICE_PORT", "50051")

	// GITHUB
	viper.SetDefault("GITHUB_CLIENT_ID", "")
	viper.SetDefault("GITHUB_CLIENT_SECRET", "")
	viper.SetDefault("GITHUB_REDIRECT_URL", "")
	viper.SetDefault("GITHUB_AUTH_URL", "https://github.com/login/oauth/authorize")
	viper.SetDefault("GITHUB_TOKEN_URL", "https://github.com/login/oauth/access_token")
	viper.SetDefault("GITHUB_SCOPES", []string{"user:email"})
	viper.SetDefault("FRONTEND_URL", "http://localhost:3000")

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
	viper.SetDefault("ALLOWED_ORIGINS", "http://localhost:3000")
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

	// Viper unmarshal không tự map flat env vars vào nested struct
	// nên phải load thủ công cho OAuth providers
	config.GitHub = GithubOAuthConfig{
		ClientID:     viper.GetString("GITHUB_CLIENT_ID"),
		ClientSecret: viper.GetString("GITHUB_CLIENT_SECRET"),
		RedirectURL:  viper.GetString("GITHUB_REDIRECT_URL"),
		Scopes:       viper.GetStringSlice("GITHUB_SCOPES"),
	}
	config.GitHub.Endpoint.AuthURL = viper.GetString("GITHUB_AUTH_URL")
	config.GitHub.Endpoint.TokenURL = viper.GetString("GITHUB_TOKEN_URL")

	config.Google = GoogleOAuthConfig{
		ClientID:     viper.GetString("GOOGLE_CLIENT_ID"),
		ClientSecret: viper.GetString("GOOGLE_CLIENT_SECRET"),
		RedirectURL:  viper.GetString("GOOGLE_REDIRECT_URL"),
	}

	config.Facebook = FacebookOAuthConfig{
		ClientID:     viper.GetString("FACEBOOK_CLIENT_ID"),
		ClientSecret: viper.GetString("FACEBOOK_CLIENT_SECRET"),
		RedirectURL:  viper.GetString("FACEBOOK_REDIRECT_URL"),
	}

	return config, nil
}
