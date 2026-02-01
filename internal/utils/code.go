package utils

import (
	"fmt"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

func GenerateUniqueCode(length int) string {
	if length < 10 {
		length = 15
	}
	if length > 30 {
		length = 20
	}

	alphabet := "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	id, err := gonanoid.Generate(alphabet, length)
	if err != nil {
		id, _ = gonanoid.New(length)
	}

	return id
}

func GenerateShortCode(length int) string {
	id, err := gonanoid.New(length)
	if err != nil {
		return ""
	}
	return id
}

func GenerateCompactCode() string {
	alphabet := "123456789ABCDEFGHJKLMNPQRSTUVWXYZ"
	id, err := gonanoid.Generate(alphabet, 15)
	if err != nil {
		id, _ = gonanoid.New(15)
	}
	return id
}
func GeneratePaymentCode() string {
	alphabet := "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
	id, err := gonanoid.Generate(alphabet, 18)
	if err != nil {
		id, _ = gonanoid.New(18)
	}
	return id
}

func GenerateTimestampBasedCode() string {
	now := time.Now()
	timestamp := now.Format("060102150405")

	nanoid, _ := gonanoid.Generate("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ", 8)

	return timestamp + nanoid
}

func ParseCodeTimestamp(code string) (time.Time, error) {
	if len(code) >= 12 {
		timestamp := code[:12]
		t, err := time.Parse("060102150405", timestamp)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot extract timestamp from code")
}
