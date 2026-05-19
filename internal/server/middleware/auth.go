package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	secretKey     []byte
	secretKeyOnce sync.Once
)

// SetJWTSecret sets the signing key. Called once at startup.
func SetJWTSecret(key []byte) {
	secretKeyOnce.Do(func() {
		secretKey = key
	})
}

type Claims struct {
	UserID         uint   `json:"user_id"`
	Username       string `json:"username"`
	MustChangePass bool   `json:"must_change_pass"`
	ExpiresAt      int64  `json:"exp"`
}

func GenerateToken(userID uint, username string, mustChangePass bool) (string, error) {
	claims := Claims{
		UserID:         userID,
		Username:       username,
		MustChangePass: mustChangePass,
		ExpiresAt:      time.Now().Add(24 * time.Hour).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)

	mac := hmac.New(sha256.New, secretKey)
	mac.Write([]byte(payloadB64))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return payloadB64 + "." + sig, nil
}

func parseToken(token string) (*Claims, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid token format")
	}

	mac := hmac.New(sha256.New, secretKey)
	mac.Write([]byte(parts[0]))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[1]), []byte(expectedSig)) {
		return nil, fmt.Errorf("invalid signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid payload encoding")
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("invalid claims")
	}

	if time.Now().Unix() > claims.ExpiresAt {
		return nil, fmt.Errorf("token expired")
	}

	return &claims, nil
}

// AuthRequired is a Gin middleware that validates JWT in Authorization header.
// Routes listed in skipPaths are exempted.
func AuthRequired(skipPaths ...string) gin.HandlerFunc {
	skip := make(map[string]bool)
	for _, p := range skipPaths {
		skip[p] = true
	}
	return func(c *gin.Context) {
		if skip[c.Request.URL.Path] {
			c.Next()
			return
		}

		auth := c.GetHeader("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid authorization header"})
			return
		}

		claims, err := parseToken(strings.TrimPrefix(auth, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("must_change_pass", claims.MustChangePass)
		c.Next()
	}
}
