package service

import (
	"os"
	"testing"

	"study.com/v1/internal/testutil/pgtest"
)

// TestMain xoá các schema Postgres dùng lại (pgtest.ReusableSchema) mà test của gói đã dựng.
func TestMain(m *testing.M) {
	code := m.Run()
	pgtest.CloseReusable()
	os.Exit(code)
}
