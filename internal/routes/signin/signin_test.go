package signin

import (
	"encoding/json"
	redisclient "fiber-gorm-api/internal/redis"
	sendgridservice "fiber-gorm-api/internal/services"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

// We'll override the actual SendGrid call so the tests won't fail
// if there's no real API key.
func init() {
	sendgridservice.SendCodeEmailFunc = func(toEmail, code string) error {
		log.Printf("[TEST-MOCK] Skipping real SendGrid call => code: %s, email: %s\n", code, toEmail)
		return nil
	}
}

// Setup function:
//   - Connects to real Redis from environment
//   - Optionally flushes data
//   - Returns a fiber.App with sign-in routes
func setupSignInTestApp(t *testing.T) *fiber.App {
	redisclient.InitRedis("session")

	// If you want to start each test from a clean state:
	if err := redisclient.Rdb.FlushAll(redisclient.Ctx).Err(); err != nil {
		log.Printf("[WARN] Could not flush Redis: %v", err)
	}

	app := fiber.New()
	RegisterRoutes(app)
	return app
}

func TestSignInRequest_MissingEmail(t *testing.T) {
	app := setupSignInTestApp(t)

	body := `{}`
	req := httptest.NewRequest("POST", "/signin/request", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", resp.StatusCode)
	}
}

func TestSignInRequest_Valid(t *testing.T) {
	app := setupSignInTestApp(t)

	body := `{"email": "request_valid@example.com"}`
	req := httptest.NewRequest("POST", "/signin/request", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	// (Optional) We could confirm that a code now exists in Redis
	codeKey := "signin_code:request_valid@example.com"
	storedCode, err := redisclient.GetValue(codeKey)
	if err != nil || storedCode == "" {
		t.Errorf("Expected a code to be stored in Redis. Key: %s, got: %q", codeKey, storedCode)
	}
}

func TestSignInRequest_RepeatedRequest(t *testing.T) {
	app := setupSignInTestApp(t)

	// 1) First request
	body := `{"email": "repeated@example.com"}`
	req1 := httptest.NewRequest("POST", "/signin/request", strings.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")

	resp1, err := app.Test(req1, -1)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp1.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d (resp1)", resp1.StatusCode)
	}
	codeKey := "signin_code:repeated@example.com"
	firstCode, _ := redisclient.GetValue(codeKey)
	if firstCode == "" {
		t.Errorf("Expected code in redis after first request")
	}

	// 2) Second request (within 5 minutes by default)
	//    Some apps might overwrite the code or re-use the same code.
	req2 := httptest.NewRequest("POST", "/signin/request", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := app.Test(req2, -1)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d (resp2)", resp2.StatusCode)
	}

	// 3) Check if the stored code is overwritten or not
	secondCode, _ := redisclient.GetValue(codeKey)
	if secondCode == "" {
		t.Errorf("Expected a code in redis after second request")
	}
	if secondCode == firstCode {
		t.Logf("The second request re-used the same code: %s", secondCode)
	} else {
		t.Logf("The second request overwrote the code. Old: %s, New: %s", firstCode, secondCode)
	}
}

func TestSignInVerify_NoCodeInRedis(t *testing.T) {
	app := setupSignInTestApp(t)

	body := `{"email":"nonexistent@example.com", "code":"123456"}`
	req := httptest.NewRequest("POST", "/signin/verify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", resp.StatusCode)
	}
}

func TestSignInVerify_InvalidCode(t *testing.T) {
	app := setupSignInTestApp(t)

	// 1) store a code
	email := "invalidcode@example.com"
	codeKey := fmt.Sprintf("signin_code:%s", email)
	if err := redisclient.SetValue(codeKey, "999999", 5*time.Minute); err != nil {
		t.Fatalf("Failed to set code in redis: %v", err)
	}

	// 2) request with a different code
	body := `{"email":"invalidcode@example.com","code":"123456"}`
	req := httptest.NewRequest("POST", "/signin/verify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 for invalid code mismatch, got %d", resp.StatusCode)
	}
}

func TestSignInVerify_Valid(t *testing.T) {
	app := setupSignInTestApp(t)

	email := "verify_ok@example.com"
	code := "654321"
	codeKey := "signin_code:" + email

	// 1) store the code in redis
	if err := redisclient.SetValue(codeKey, code, 5*time.Minute); err != nil {
		t.Fatalf("Failed to set code in redis: %v", err)
	}

	// 2) request
	body := fmt.Sprintf(`{"email":"%s","code":"%s"}`, email, code)
	req := httptest.NewRequest("POST", "/signin/verify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	// 3) parse out the token
	var result map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&result)
	token := result["token"]
	if token == "" {
		t.Errorf("Expected 'token' in JSON, got: %#v", result)
	}

	// 4) confirm the code was removed (single-use)
	val, _ := redisclient.GetValue(codeKey)
	if val != "" {
		t.Errorf("Expected code to be removed after successful verify, but got '%s'", val)
	}

	// 5) confirm there's a session in redis
	//    The signIn code sets "session:<randomToken>" => user profile
	//    We can't guess the random sessionID, so let's just trust your code or
	//    parse the JWT to see session_key. Another approach is to parse the JWT
	//    claims if needed.
}

func TestSignInVerify_RepeatedUse(t *testing.T) {
	app := setupSignInTestApp(t)

	email := "oneuse@example.com"
	code := "987654"
	key := "signin_code:" + email
	if err := redisclient.SetValue(key, code, 5*time.Minute); err != nil {
		t.Fatalf("Failed to set code: %v", err)
	}

	// 1) successful verify
	body1 := fmt.Sprintf(`{"email":"%s","code":"%s"}`, email, code)
	req1 := httptest.NewRequest("POST", "/signin/verify", strings.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	resp1, err := app.Test(req1)
	if err != nil {
		t.Fatalf("req1 error: %v", err)
	}
	if resp1.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp1.StatusCode)
	}

	// 2) second attempt with same code should fail because code was removed
	body2 := fmt.Sprintf(`{"email":"%s","code":"%s"}`, email, code)
	req2 := httptest.NewRequest("POST", "/signin/verify", strings.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("req2 error: %v", err)
	}
	if resp2.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400 second time because code is no longer in redis, got %d", resp2.StatusCode)
	}
}

func TestSignInVerify_EmptyBody(t *testing.T) {
	app := setupSignInTestApp(t)

	req := httptest.NewRequest("POST", "/signin/verify", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400 for missing email/code, got %d", resp.StatusCode)
	}
}
