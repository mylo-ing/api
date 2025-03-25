package middleware

import (
	"encoding/json"
	redisclient "fiber-gorm-api/internal/redis"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// We'll define a minimal next handler for our tests.
// If it gets called, it sets a "nextCalled" key in locals.
func nextHandler(c *fiber.Ctx) error {
	c.Locals("nextCalled", true)
	return c.JSON(fiber.Map{"message": "Welcome, you have a valid token"})
}

// Setup a fiber app that uses RequireJWT and nextHandler
// so we can test different token scenarios.
func setupJWTTestApp() *fiber.App {
	redisclient.InitRedis("session") // rely on real Redis from Docker Compose

	app := fiber.New()
	app.Use(RequireJWT)

	app.Get("/test-jwt", nextHandler)
	return app
}

func TestNoToken(t *testing.T) {
	app := setupJWTTestApp()

	req := httptest.NewRequest("GET", "/test-jwt", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 for missing token, got %d", resp.StatusCode)
	}
}

func TestInvalidTokenFormat(t *testing.T) {
	app := setupJWTTestApp()

	// Put the token directly, no "Bearer " prefix
	req := httptest.NewRequest("GET", "/test-jwt", nil)
	req.Header.Set("Authorization", "InvalidToken")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 for invalid format, got %d", resp.StatusCode)
	}
}

func TestMalformedToken(t *testing.T) {
	app := setupJWTTestApp()

	req := httptest.NewRequest("GET", "/test-jwt", nil)
	req.Header.Set("Authorization", "Bearer abc.def.ghi") // random malformed token

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 for malformed token, got %d", resp.StatusCode)
	}
}

func TestExpiredToken(t *testing.T) {
	app := setupJWTTestApp()

	// Manually create a token that is already expired
	secret := os.Getenv("JWT_USER_SECRET_KEY")
	if secret == "" {
		secret = "devsecret"
	}

	claims := jwt.MapClaims{
		"session_key": "expiredSessionKey",
		"exp":         jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)), // 1 hour ago
		"iat":         jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, _ := token.SignedString([]byte(secret))

	req := httptest.NewRequest("GET", "/test-jwt", nil)
	req.Header.Set("Authorization", "Bearer "+ss)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 for expired token, got %d", resp.StatusCode)
	}
}

func TestValidTokenNoSession(t *testing.T) {
	app := setupJWTTestApp()

	// Generate a valid token, but the session doesn't exist in Redis
	ss, err := generateTestJWT("nonexistentSessionKey")
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	req := httptest.NewRequest("GET", "/test-jwt", nil)
	req.Header.Set("Authorization", "Bearer "+ss)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 because session key not found, got %d", resp.StatusCode)
	}
}

func TestValidTokenWithSession(t *testing.T) {
	app := setupJWTTestApp()

	// 1) Create a session in Redis
	sessionID := "validSessionTest"
	userProfile := `{"email":"valid@example.com"}`
	if err := redisclient.SetValue("session:"+sessionID, userProfile, 0); err != nil {
		t.Fatalf("failed to store session in redis: %v", err)
	}

	// 2) Generate a valid token referencing that session
	ss, err := generateTestJWT(sessionID)
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	// 3) Make request with token
	req := httptest.NewRequest("GET", "/test-jwt", nil)
	req.Header.Set("Authorization", "Bearer "+ss)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	// optionally parse response
	var data map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&data)
	if data["message"] != "Welcome, you have a valid token" {
		t.Errorf("Expected success message, got: %v", data)
	}
}

// TestGenerateJWT checks if the function sets session_key, exp, iat
func TestGenerateJWT(t *testing.T) {
	token, err := GenerateJWT("someSessionKey")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}
	if token == "" {
		t.Fatal("Expected non-empty token")
	}

	secret := os.Getenv("JWT_USER_SECRET_KEY")
	if secret == "" {
		secret = "devsecret"
	}

	parsed, err := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		t.Fatalf("Failed to parse generated token: %v", err)
	}
	if !parsed.Valid {
		t.Errorf("Generated token is not valid")
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatalf("Claims are not MapClaims")
	}

	sess, ok := claims["session_key"]
	if !ok || sess != "someSessionKey" {
		t.Errorf("Expected session_key=someSessionKey, got %v", sess)
	}
}

// helper to generate a test token referencing a sessionKey
func generateTestJWT(sessionKey string) (string, error) {
	secret := os.Getenv("JWT_USER_SECRET_KEY")
	if secret == "" {
		secret = "devsecret"
	}

	now := time.Now()
	exp := now.Add(time.Hour) // valid for 1 hour

	claims := jwt.MapClaims{
		"session_key": sessionKey,
		"exp":         jwt.NewNumericDate(exp),
		"iat":         jwt.NewNumericDate(now),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}
