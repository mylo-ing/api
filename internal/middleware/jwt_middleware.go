package middleware

import (
	"os"
	"strings"
	"time"

	redisclient "fiber-gorm-api/internal/redis"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// RequireJWT is a Fiber middleware that checks for a valid JWT in Authorization header
func RequireJWT(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing Authorization header"})
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token format"})
	}

	claims := jwt.MapClaims{}
	secret := os.Getenv("JWT_USER_SECRET_KEY")
	if secret == "" {
		secret = "devsecret"
	}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid or expired token"})
	}

	sessionKey, ok := claims["session_key"].(string)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Session key missing in token"})
	}

	// Check Redis for session
	sessionVal, err := redisclient.GetValue("session:" + sessionKey)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Session not found or expired"})
	}
	if sessionVal == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Session invalid or not found"})
	}

	return c.Next()
}

// GenerateJWT creates a new JWT with the given session key, valid for 1 day
func GenerateJWT(sessionKey string) (string, error) {
	secret := os.Getenv("JWT_USER_SECRET_KEY")
	if secret == "" {
		secret = "devsecret"
	}

	// Use explicit time.Now() instead of jwt.TimeFunc
	now := time.Now()
	exp := now.Add(24 * time.Hour)

	claims := jwt.MapClaims{
		"session_key": sessionKey,
		"exp":         jwt.NewNumericDate(exp),
		"iat":         jwt.NewNumericDate(now),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", err
	}
	return ss, nil
}
