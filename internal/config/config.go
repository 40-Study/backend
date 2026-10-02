package config

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/shopspring/decimal"
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

	MinioHost           string `mapstructure:"MINIO_HOST"`
	MinioPort           string `mapstructure:"MINIO_PORT"`
	MinioAccessKey      string `mapstructure:"MINIO_ACCESS_KEY"`
	MinioSecretKey      string `mapstructure:"MINIO_SECRET_KEY"`
	MinioUseSSL         bool   `mapstructure:"MINIO_USE_SSL"`
	MinioPublicEndpoint string `mapstructure:"MINIO_PUBLIC_ENDPOINT"` // Public URL for presigned URLs (e.g., https://cdn.fortex.ai.vn)

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

	// HLSSigningSecret: secret HMAC ký URL ngắn hạn cho /api/hls/* (xem package hlsauth). Bắt buộc,
	// KHÔNG có mặc định — kiểm ở app.InitResources (không kiểm trong LoadConfig để cmd/seed, vốn
	// không phát video, vẫn chạy được mà không cần secret này).
	HLSSigningSecret string `mapstructure:"HLS_SIGNING_SECRET"`

	// CORS
	AllowedOrigins string `mapstructure:"ALLOWED_ORIGINS"`

	// TrustedProxies (review PR #69 BLOCKER, QA 260927): danh sách IP/CIDR của các hop đứng
	// TRƯỚC Fiber mà server tin tưởng để đọc header X-Forwarded-For thay vì TCP peer trực tiếp —
	// xem ResolvedTrustedProxies() để biết vì sao cần field này.
	TrustedProxies string `mapstructure:"TRUSTED_PROXIES"`

	// Transaction Service (MBBank gRPC)
	TransactionServiceHost string `mapstructure:"TRANSACTION_SERVICE_HOST"`
	TransactionServicePort string `mapstructure:"TRANSACTION_SERVICE_PORT"`
	// TransactionServiceToken (S6): secret dùng chung với service Python — gửi trong metadata gRPC
	// "x-transaction-token". Rỗng = không gửi (tương thích service chưa bật xác thực).
	TransactionServiceToken string `mapstructure:"TRANSACTION_SERVICE_TOKEN"`
	// PaymentReconcileIntervalMinutes (L6 mục 5): chu kỳ (phút) của job nền đối chiếu ngân hàng cho đơn
	// chờ và đơn vừa hoàn tất. Mặc định 10; hợp lệ 1-59 (cron theo phút), ngoài khoảng thì
	// LoadConfig báo lỗi lúc khởi động thay vì lặng lẽ đổi giá trị.
	PaymentReconcileIntervalMinutes int `mapstructure:"PAYMENT_RECONCILE_INTERVAL_MINUTES"`

	// OAuth Providers
	GitHub   GithubOAuthConfig
	Google   GoogleOAuthConfig
	Facebook FacebookOAuthConfig

	// Frontend URL for OAuth redirect
	FrontendURL                string `mapstructure:"FRONTEND_URL"`
	ParentInvitationDailyLimit int    `mapstructure:"PARENT_INVITATION_DAILY_LIMIT"` // Số lần gửi lời mời phụ huynh tối đa trong 24 giờ của mỗi học sinh
	TempDir                    string `mapstructure:"TEMP_DIR"`                      // Thư mục tạm để xử lý video, nên đặt ở ổ đĩa có dung lượng lớn và tốc độ cao (ví dụ: D:\temp\video-processing)

	// Phase 4 rút tiền giảng viên (quyết định chủ dự án #7: tối thiểu 100.000đ, cấu hình được).
	// Raw là chuỗi đọc từ env WITHDRAWAL_MIN_AMOUNT; WithdrawalMinAmount là giá trị đã parse +
	// validate trong LoadConfig (sai định dạng hoặc <= 0 thì dừng khởi động, không lặng lẽ dùng mặc định).
	WithdrawalMinAmountRaw string          `mapstructure:"WITHDRAWAL_MIN_AMOUNT"`
	WithdrawalMinAmount    decimal.Decimal `mapstructure:"-"`
}

// DefaultWithdrawalMinAmount — mặc định của WITHDRAWAL_MIN_AMOUNT (VND).
const DefaultWithdrawalMinAmount = "100000"

// DefaultPaymentReconcileIntervalMinutes — mặc định của PAYMENT_RECONCILE_INTERVAL_MINUTES.
const DefaultPaymentReconcileIntervalMinutes = 10

// validatePaymentReconcileInterval là hàm thuần để unit test: 1-59 phút (cron `*/N * * * *`).
func validatePaymentReconcileInterval(minutes int) (int, error) {
	if minutes < 1 || minutes > 59 {
		return 0, fmt.Errorf("PAYMENT_RECONCILE_INTERVAL_MINUTES must be between 1 and 59, got %d", minutes)
	}
	return minutes, nil
}

// parseWithdrawalMinAmount là hàm thuần để unit test: chỉ chấp nhận số dương.
func parseWithdrawalMinAmount(raw string) (decimal.Decimal, error) {
	v, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil {
		return decimal.Zero, fmt.Errorf("WITHDRAWAL_MIN_AMOUNT %q is not a number: %w", raw, err)
	}
	if !v.IsPositive() {
		return decimal.Zero, fmt.Errorf("WITHDRAWAL_MIN_AMOUNT must be greater than 0, got %s", v)
	}
	return v, nil
}

func LoadConfig() (*Config, error) {
	var env string
	defaultEnv := strings.ToLower(strings.TrimSpace(os.Getenv("ENVIRONMENT")))
	if defaultEnv == "" {
		defaultEnv = "dev"
	}
	if defaultEnv == "production" {
		defaultEnv = "prod"
	}
	flag.StringVar(&env, "env", defaultEnv, "Environment (dev, test, prod)")
	flag.Parse()
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "production" {
		env = "prod"
	}

	config := &Config{}
	viper.Set("ENVIRONMENT", env)

	// Set default values
	viper.SetDefault("PORT", "5000")
	viper.SetDefault("HOST", "localhost")
	viper.SetDefault("REDIS_DB", 0)
	viper.SetDefault("MINIO_USE_SSL", false)
	viper.SetDefault("MINIO_BUCKET_IMAGES", "images")
	viper.SetDefault("MINIO_BUCKET_VIDEOS", "videos")
	viper.SetDefault("MINIO_BUCKET_NAME", "videos")

	// Transaction Service
	viper.SetDefault("TRANSACTION_SERVICE_HOST", "localhost")
	viper.SetDefault("TRANSACTION_SERVICE_PORT", "50051")
	viper.SetDefault("TRANSACTION_SERVICE_TOKEN", "")
	viper.SetDefault("PAYMENT_RECONCILE_INTERVAL_MINUTES", DefaultPaymentReconcileIntervalMinutes)

	// GITHUB
	viper.SetDefault("GITHUB_CLIENT_ID", "")
	viper.SetDefault("GITHUB_CLIENT_SECRET", "")
	viper.SetDefault("GITHUB_REDIRECT_URL", "")
	viper.SetDefault("GITHUB_AUTH_URL", "https://github.com/login/oauth/authorize")
	viper.SetDefault("GITHUB_TOKEN_URL", "https://github.com/login/oauth/access_token")
	viper.SetDefault("GITHUB_SCOPES", []string{"user:email"})
	viper.SetDefault("FRONTEND_URL", "http://localhost:3000")
	viper.SetDefault("WITHDRAWAL_MIN_AMOUNT", DefaultWithdrawalMinAmount)

	viper.AutomaticEnv()

	// Load appropriate .env file based on environment
	var configFile string
	if env == "prod" {
		configFile = ".env.prod"
	} else {
		configFile = ".env"
	}

	viper.SetConfigType("env")
	viper.SetConfigFile(configFile)

	if err := viper.ReadInConfig(); err != nil {
		// It's okay if config file doesn't exist, we might rely on env vars
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		fmt.Printf("No %s file found, relying on environment variables\n", configFile)
	} else {
		fmt.Printf("Loaded configuration from %s file\n", configFile)
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
	// H-04 (audit 260909 vòng 2): trước đây có viper.SetDefault("JWT_SECRET",
	// "supersecretkey-change-in-production") — nếu deploy thiếu biến môi trường JWT_SECRET,
	// app vẫn khởi động bình thường và ký/verify token bằng 1 secret CÔNG KHAI (nằm thẳng
	// trong source code), cho phép bất kỳ ai tự ký JWT hợp lệ giả danh user/admin bất kỳ.
	// Xoá default — JWT_SECRET bắt buộc phải được set qua .env/.env.prod hoặc biến môi
	// trường thật; validate ngay dưới đây để fail-fast khi thiếu/còn giá trị mặc định cũ.
	// Đăng ký key với viper (giá trị rỗng, KHÔNG phải secret mặc định) để Unmarshal thấy biến môi
	// trường HLS_SIGNING_SECRET; giá trị rỗng bị hlsauth.ValidateSecret từ chối lúc khởi động.
	viper.SetDefault("HLS_SIGNING_SECRET", "")
	viper.SetDefault("ALLOWED_ORIGINS", "http://localhost:3000")
	viper.SetDefault("JWT_ACCESS_EXPIRATION_MINUTES", 15)
	viper.SetDefault("JWT_REFRESH_EXPIRATION_DAYS", 7)

	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("unable to decode into struct: %w", err)
	}

	// H-04: fail-fast thay vì âm thầm chạy với secret rỗng hoặc secret mặc định cũ đã từng
	// nằm trong source code (nếu deploy nào đó copy nguyên .env mẫu cũ có giá trị này).
	// Tách thành hàm thuần validateJWTSecret để unit test được (LoadConfig dùng flag/viper
	// global state, gọi 2 lần trong 1 process test sẽ panic "flag redefined").
	if err := validateJWTSecret(config.JWTSecret, configFile); err != nil {
		return nil, err
	}

	minWithdrawal, err := parseWithdrawalMinAmount(config.WithdrawalMinAmountRaw)
	if err != nil {
		return nil, err
	}
	config.WithdrawalMinAmount = minWithdrawal

	reconcileMinutes, err := validatePaymentReconcileInterval(config.PaymentReconcileIntervalMinutes)
	if err != nil {
		return nil, err
	}
	config.PaymentReconcileIntervalMinutes = reconcileMinutes

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

// insecureDefaultJWTSecret là giá trị mặc định KHÔNG AN TOÀN đã từng nằm thẳng trong source
// code trước H-04 (audit 260909 vòng 2) — nếu bất kỳ deploy nào copy nguyên file .env mẫu cũ
// (chứa chuỗi này), token vẫn ký được bằng secret công khai. Giữ hằng số riêng (không phải
// literal lặp lại) để dễ grep/audit sau này.
const insecureDefaultJWTSecret = "supersecretkey-change-in-production"

// validateJWTSecret (H-04) là hàm THUẦN (không đụng viper/flag/env) để unit test được độc
// lập — LoadConfig() dùng global flag.CommandLine nên gọi LoadConfig() nhiều lần trong 1
// process test sẽ panic "flag redefined: env"; tách validate ra khỏi đó tránh vấn đề này.
func validateJWTSecret(secret, configFileHint string) error {
	if strings.TrimSpace(secret) == "" {
		return fmt.Errorf("JWT_SECRET is required but not set — set it in %s or as an environment variable", configFileHint)
	}
	if secret == insecureDefaultJWTSecret {
		return fmt.Errorf("JWT_SECRET is still set to the old insecure default value — set a real secret in %s or as an environment variable", configFileHint)
	}
	return nil
}

// DefaultAllowedOrigins (N4, review vong 2 260915): gia tri mac dinh SSOT khi ALLOWED_ORIGINS
// khong duoc dat trong .env — truoc day chuoi "http://localhost:3000" bi LAP o hai noi
// (internal/app/app.go cho middleware cors.New, internal/router/enrollment_router.go cho
// middleware.SameOriginRequired). Doi mot noi ma quen doi noi kia thi hai middleware nay se
// khong con dung chung mot default nua ma khong co dau hieu bao loi nao.
const DefaultAllowedOrigins = "http://localhost:3000"

// ResolvedAllowedOrigins tra ve ALLOWED_ORIGINS da fallback ve DefaultAllowedOrigins khi rong,
// va log canh bao MOT LAN moi lan goi neu gia tri la "*". An toan goi tren con tro nil (tra ve
// thang default) de cac noi mount router trong test (cfg=nil, xem enrollment_router_test.go)
// khong can tu kiem nil truoc khi goi.
//
// "*" tat HOAN TOAN ca cors.New lan middleware.SameOriginRequired (chan CSRF cho
// POST /api/progress) — khong co dau hieu nao khac luc khoi dong, nen mot file .env production
// dat gia tri nay se mat lop bao ve CSRF ma khong ai biet cho toi khi bi khai thac.
func (c *Config) ResolvedAllowedOrigins() string {
	if c == nil || c.AllowedOrigins == "" {
		return DefaultAllowedOrigins
	}
	if c.AllowedOrigins == "*" {
		log.Printf("[WARN] ALLOWED_ORIGINS=\"*\" — CORS mo hoan toan VA middleware.SameOriginRequired (chan CSRF cho POST /api/progress) bi TAT HOAN TOAN. Chi dung gia tri nay o moi truong dev, khong dung production.")
	}
	return c.AllowedOrigins
}

// DefaultTrustedProxies (review PR #69 BLOCKER, QA 260927): mac dinh khi TRUSTED_PROXIES rong —
// CHI localhost (nginx/Next.js chay tren cung may trong dev, hoac reverse-proxy tren cung host
// trong mot so hạ tang). Production dung Next.js server o may/container khac PHAI dat
// TRUSTED_PROXIES that trong .env, neu khong X-Forwarded-For se bi bo qua va rate-limit quay ve
// dung TCP peer (dung nhung khong phan biet duoc user that voi nhau qua proxy — xem
// ResolvedTrustedProxies).
const DefaultTrustedProxies = "127.0.0.1,::1"

// ResolvedTrustedProxies (review PR #69 BLOCKER, QA 260927): danh sach IP/CIDR "hop cuoi cung
// truoc Fiber" ma server duoc PHEP tin de doc X-Forwarded-For thay vi TCP peer.
//
// Ly do can field nay: web goi backend qua proxy server-side cua Next.js
// (web/src/app/api/[...path]/route.ts) — trinh duyet luon noi voi Next.js truoc, roi Next.js
// (khong phai trinh duyet) moi la ben THAT SU mo ket noi TCP toi backend. Neu Fiber khong duoc
// cau hinh doc X-Forwarded-For, moi request tu MOI nguoi dung that deu bi Fiber nhin thanh CUNG
// MOT TCP peer (IP cua tien trinh Next.js) — cac rate limiter theo IP (AuthRateLimiter,
// WideAuthRateLimiter, OTPRateLimiter) vi vay chia se DUY NHAT 1 bucket cho toan bo website,
// khong phai rieng cho tung nguoi dung/mang nhu PR S-P1-2 (QA 260927) tuong nham.
//
// EnableTrustedProxyCheck=true bat buoc Fiber CHI doc X-Forwarded-For khi TCP peer THAT SU nam
// trong danh sach nay — neu khong, bat ky client nao cung tu xung IP gia qua header de né rate
// limit hoac mao danh IP nguoi khac. KHONG duoc bat ProxyHeader ma khong co danh sach nay (hoac
// de rong ma khong xac nhan reverse-proxy that su dung dung dia chi trong danh sach).
func (c *Config) ResolvedTrustedProxies() []string {
	raw := DefaultTrustedProxies
	if c != nil && strings.TrimSpace(c.TrustedProxies) != "" {
		raw = c.TrustedProxies
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
