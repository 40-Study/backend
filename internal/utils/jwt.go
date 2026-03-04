package utils

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"study.com/v1/internal/config"
)

type Claims struct {
	UserID      uuid.UUID  `json:"user_id"`
	DeviceID    uuid.UUID  `json:"device_id"`
	ActiveRole  string     `json:"active_role"`
	ActiveOrgID *uuid.UUID `json:"active_org_id,omitempty"`
	UserVersion int64      `json:"user_version"`
	jwt.RegisteredClaims
}

func GenerateTokens(cfg *config.Config, userID uuid.UUID, deviceID uuid.UUID, activeRole string, activeOrgID *uuid.UUID, userVersion int64) (string, string, error) {
	accessClaims := Claims{
		UserID:      userID,
		DeviceID:    deviceID,
		ActiveRole:  activeRole,
		ActiveOrgID: activeOrgID,
		UserVersion: userVersion,
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

	refreshClaims := Claims{
		UserID:      userID,
		DeviceID:    deviceID,
		ActiveRole:  activeRole,
		ActiveOrgID: activeOrgID,
		UserVersion: userVersion,
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
