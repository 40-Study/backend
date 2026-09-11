package main

import (
	"log"

	"github.com/shopspring/decimal"
	"study.com/v1/internal/app"
)

func main() {
	// W3: mọi field tiền/điểm (decimal.Decimal) serialize thành JSON number thay vì
	// chuỗi có dấu ngoặc kép — hợp đồng API: mọi field tiền/điểm là số, không phải string.
	decimal.MarshalJSONWithoutQuotes = true

	application, err := app.New()
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}

	// Run the application
	if err := application.Run(); err != nil {
		log.Fatalf("Failed to run application: %v", err)
	}
}
