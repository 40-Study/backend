// Creates a teacher, course, class and scheduled livestream for local QA.
package main

import (
	"encoding/json"
	"fmt"
	"gorm.io/gorm/logger"
	"log"
	"strings"
	"study.com/v1/internal/config"
	"study.com/v1/internal/database"
	"study.com/v1/internal/database/seeds"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	env := strings.ToLower(cfg.Environment)
	if env == "prod" || env == "production" {
		log.Fatal("demo seed is local development only")
	}
	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatal(err)
	}
	db.Logger = logger.Default.LogMode(logger.Silent)
	out, err := seeds.NewSeeder(db).SeedLivestreamWorkspace()
	if err != nil {
		log.Fatal(err)
	}
	data, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(data))
}
