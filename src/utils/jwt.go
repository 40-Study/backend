package utils

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"tiger.com/v2/src/config"
)

type Claims struct {
	UserID   uuid.UUID `json:"user_id"`
	DeviceID uuid.UUID `json:"device_id"`
	Version  int64     `json:"version"`
	jwt.RegisteredClaims
}

func GenerateTokens(cfg *config.Config, userID uuid.UUID, deviceID uuid.UUID, version int64) (string, string, error) {
	// Access Token
	accessClaims := Claims{
		UserID:   userID,
		DeviceID: deviceID,
		Version:  version,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(cfg.JWTAccessExpiration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		return "", "", err
	}

	// Refresh Token
	refreshClaims := Claims{
		UserID:   userID,
		DeviceID: deviceID,
		Version:  version,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(cfg.JWTRefreshExpiration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		return "", "", err
	}

	return accessTokenString, refreshTokenString, nil
}

func ParseToken(cfg *config.Config, tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(cfg.JWTSecret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}
