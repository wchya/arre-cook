package auth

import (
	"ninimenu/internal/config"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestTokenRequiresExpiryAndRejectsFutureIssueTime(t *testing.T) {
	old := config.C.JWTSecret
	config.C.JWTSecret = "isolated-token-validation-test-secret"
	t.Cleanup(func() { config.C.JWTSecret = old })
	now := time.Now()
	for _, row := range []struct {
		expires, issued *jwt.NumericDate
		valid           bool
	}{
		{nil, jwt.NewNumericDate(now), false},
		{jwt.NewNumericDate(now.Add(-time.Minute)), jwt.NewNumericDate(now.Add(-time.Hour)), false},
		{jwt.NewNumericDate(now.Add(time.Hour)), jwt.NewNumericDate(now.Add(time.Minute)), false},
		{jwt.NewNumericDate(now.Add(time.Hour)), jwt.NewNumericDate(now), true},
	} {
		raw, err := sign(Claims{Kind: KindUser, RegisteredClaims: jwt.RegisteredClaims{Issuer: issuer, Subject: "1", ExpiresAt: row.expires, IssuedAt: row.issued}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = ParseToken(raw)
		if (err == nil) != row.valid {
			t.Fatalf("valid=%v, error=%v", row.valid, err)
		}
	}
}
