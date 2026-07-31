// Command seed nạp dữ liệu demo vào database để phát triển và trình diễn.
//
//	go run ./cmd/seed              # seed roles/permissions + dữ liệu demo
//	go run ./cmd/seed -mode=base   # chỉ seed permissions + system roles
package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"study.com/v1/internal/config"
	"study.com/v1/internal/database"
	"study.com/v1/internal/database/seeds"
)

func validateSeedTarget(environment, mode string, allowProduction bool) error {
	switch mode {
	case "base", "full":
	default:
		return fmt.Errorf("unsupported seed mode %q (expected base or full)", mode)
	}

	env := strings.ToLower(strings.TrimSpace(environment))
	if (env == "prod" || env == "production") && !allowProduction {
		return fmt.Errorf("refusing to seed production without -allow-production")
	}
	return nil
}

func main() {
	// config.LoadConfig gọi flag.Parse nội bộ, nên khai báo cờ trước khi load.
	mode := flag.String("mode", "full", "Chế độ seed: full (roles + demo data) hoặc base (chỉ roles/permissions)")
	dataDir := flag.String("data", "./data", "Thư mục chứa roles.json và permissions/")
	allowProduction := flag.Bool("allow-production", false, "Cho phép seed vào production (nguy hiểm)")

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if err := validateSeedTarget(cfg.Environment, *mode, *allowProduction); err != nil {
		log.Fatalf("Unsafe seed request: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	if err := database.Migrate(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	seeder := seeds.NewSeeder(db)

	if err := seeder.SeedAll(*dataDir); err != nil {
		log.Fatalf("Failed to seed roles/permissions: %v", err)
	}

	if *mode == "base" {
		log.Println("Mode=base: bỏ qua dữ liệu demo.")
		return
	}

	if err := seeder.SeedDemoData(); err != nil {
		log.Fatalf("Failed to seed demo data: %v", err)
	}

	log.Printf("Done. Tài khoản demo dùng mật khẩu: %s", seeds.DemoPassword)
}
