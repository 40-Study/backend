package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"math/big"
)

func GenerateOTP(length int) (string, error) {
	const charset = "0123456789"
	otp := make([]byte, length)
	for i := range otp {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		otp[i] = charset[num.Int64()]
	}
	return string(otp), nil
}

// HashOTP creates a SHA256 hash of the OTP with email as salt
// This prevents storing plaintext OTP in Redis
func HashOTP(otp, email string) string {
	h := sha256.New()
	h.Write([]byte(otp + ":" + email))
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyOTP compares OTP using constant-time comparison to prevent timing attacks
func VerifyOTP(inputOTP, storedHash, email string) bool {
	inputHash := HashOTP(inputOTP, email)
	return subtle.ConstantTimeCompare([]byte(inputHash), []byte(storedHash)) == 1
}

func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func GenerateSecureToken() (string, string, error) {
	b := make([]byte, 32) 
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token := hex.EncodeToString(b) // gửi qua email
	// token là mã gửi qua email, hashToken là giá trị lưu trong DB để so sánh khi người dùng nhập token
	return token, HashToken(token), nil
}
