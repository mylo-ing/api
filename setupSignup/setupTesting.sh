# Below are updated integration test files for:
#   1) The sign-in flow (Request & Verify) with real Redis usage
#   2) JWT protection on admin routes (requires a valid token + session in Redis)
#
# IMPORTANT NOTES:
# - These tests assume you are running them in a Docker Compose environment where
#   Redis is provided by the 'redis' service (or however your config is set up).
# - The code uses environment variables (REDIS_HOST, REDIS_SESSION_DB, etc.) to
#   connect to the REAL Redis instance rather than using an in-memory miniredis.
# - We flush Redis at the start of each test file to ensure a clean state.
# - We mock or allow "SendGrid" calls to pass even if you have no valid SENDGRID_API_KEY,
#   so your tests won't fail simply due to missing credentials.
# - Adjust the file paths/names as desired. Typically, you'd place them in your
#   existing test structure, e.g.:
#       internal/routes/signin/signin_test.go
#       internal/routes/admin/admin_jwt_test.go
# - Everything else (handlers, redisclient, etc.) remains as previously established.
#
######################################################################################
# FILE 1: internal/routes/signin/signin_test.go
######################################################################################
package signin

import (
	"encoding/json"
	"fiber-gorm-api/internal/redisclient"
	"fiber-gorm-api/internal/routes/signin/sendgridservice"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Override SendCodeEmail so the test won't fail if no valid SendGrid key is provided.
// In real usage, you might skip this override if you want to truly test the emailing part.
func init() {
	sendgridservice.SendCodeEmail = func(toEmail, code string) error {
		log.Printf("[TEST-MOCK] No real SendGrid call. Code=%s, Email=%s\n", code, toEmail)
		return nil
	}
}

// setupSignInTest initializes real Redis (based on environment vars) and returns a Fiber app
// with sign-in routes registered. We also flush all Redis data for a clean slate.
func setupSignInTest() *fiber.App {
	// Connect to actual Redis from Docker Compose (using env variables like REDIS_HOST, etc.)
	redisclient.InitRedis()

	// Optionally flush all keys so each test starts fresh
	if err := redisclient.Rdb.FlushAll(redisclient.Ctx).Err(); err != nil {
		log.Printf("[WARN] Could not flush Redis: %v", err)
	}

	app := fiber.New()
	RegisterRoutes(app) // sets up POST /signin/request, /signin/verify
	return app
}

func TestRequestSignInIntegration(t *testing.T) {
	app := setupSignInTest()

	t.Run("MissingEmail", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/signin/request", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("ValidRequestNoRealSendGrid", func(t *testing.T) {
		body := `{"email": "testing@example.com"}`
		req := httptest.NewRequest("POST", "/signin/request", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}
	})
}

func TestVerifySignInIntegration(t *testing.T) {
	app := setupSignInTest()

	t.Run("NoCodeInRedis", func(t *testing.T) {
		body := `{"email":"doesnotexist@example.com","code":"111111"}`
		req := httptest.NewRequest("POST", "/signin/verify", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400 when no code found, got %d", resp.StatusCode)
		}
	})

	t.Run("ValidFlow", func(t *testing.T) {
		email := "validflow@example.com"
		code := "654321"
		// store a code in Redis
		if err := redisclient.SetValue("signin_code:"+email, code, 5*time.Minute); err != nil {
			t.Fatalf("Failed to set test code in Redis: %v", err)
		}

		body := `{"email":"validflow@example.com","code":"654321"}`
		req := httptest.NewRequest("POST", "/signin/verify", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}

		var result map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&result)
		token := result["token"]
		if token == "" {
			t.Errorf("Expected 'token' in response, got none.")
		}
	})
}


######################################################################################
# FILE 2: internal/routes/admin/admin_jwt_test.go
######################################################################################
package admin

import (
	"bytes"
	"encoding/json"
	"fiber-gorm-api/internal/db"
	"fiber-gorm-api/internal/middleware"
	"fiber-gorm-api/internal/models"
	"fiber-gorm-api/internal/redisclient"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// create an ephemeral in-memory DB for subscriber data
func setupInMemoryDB() *gorm.DB {
	inMem, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	inMem.AutoMigrate(&models.Subscriber{}, &models.SubscriberType{})
	return inMem
}

// setupAdminJWTTest uses real Redis from Docker Compose, ephemeral DB for the admin routes
func setupAdminJWTTest() *fiber.App {
	// connect to real Redis
	redisclient.InitRedis()
	if err := redisclient.Rdb.FlushAll(redisclient.Ctx).Err(); err != nil {
		log.Printf("[WARN] Could not flush Redis: %v", err)
	}

	// ephemeral DB
	database := setupInMemoryDB()

	app := fiber.New()
	RegisterAdminRoutes(app, database) // includes JWT requirement
	return app
}

func TestAdminRequiresJWTIntegration(t *testing.T) {
	app := setupAdminJWTTest()

	t.Run("NoToken", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/subscribers", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("InvalidToken", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/subscribers", nil)
		req.Header.Set("Authorization", "Bearer blahblah")
		resp, _ := app.Test(req)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("ValidToken", func(t *testing.T) {
		// 1) store a session in Redis
		sessionID := "myAdminSession"
		sessionData := `{"email":"adminuser@example.com"}`
		if err := redisclient.SetValue("session:"+sessionID, sessionData, 0); err != nil {
			t.Fatalf("Failed to set session in redis: %v", err)
		}

		// 2) Generate a valid JWT referencing that session
		token, err := middleware.GenerateJWT(sessionID)
		if err != nil {
			t.Fatalf("Failed to generate token: %v", err)
		}

		req := httptest.NewRequest("GET", "/admin/subscribers", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, _ := app.Test(req)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}
	})
}

func TestAdminCreateSubscriberWithJWTIntegration(t *testing.T) {
	app := setupAdminJWTTest()

	// store session in redis
	sessionID := "admincreateTest"
	userData := `{"email":"admincreate@example.com"}`
	if err := redisclient.SetValue("session:"+sessionID, userData, 0); err != nil {
		t.Fatalf("Failed to store session: %v", err)
	}
	token, err := middleware.GenerateJWT(sessionID)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	payload := `{
		"email": "sub-user@example.com",
		"name": "Sub User",
		"subscriber_types": []
	}`

	req := httptest.NewRequest("POST", "/admin/subscribers", bytes.NewBuffer([]byte(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		body := new(strings.Builder)
		_, _ = body.ReadFrom(resp.Body)
		t.Errorf("Expected 201, got %d => %s", resp.StatusCode, body.String())
		return
	}

	var created models.Subscriber
	_ = json.NewDecoder(resp.Body).Decode(&created)
	if created.ID == 0 {
		t.Errorf("Expected new subscriber to have an ID")
	}
}

######################################################################################
# USAGE NOTES:
# 1) In your docker-compose.yml, define a `redis` service (with volumes if needed).
#    The `api` and `test` containers should have environment variables like:
#       REDIS_HOST=mylocal_redis:6379
#       REDIS_SESSION_DB=0
#       REDIS_PASSWORD=
#    etc., so they connect to that real Redis instance.
# 2) The tests connect to Redis by calling `redisclient.InitRedis()` which reads
#    those environment variables. Then we do a `FlushAll()` for a clean slate.
# 3) If you want to attempt real emails, remove the `init()` override in 
#    `signin_test.go` – or conditionally override it only if 
#    SENDGRID_API_KEY is blank.
# 4) Then run `docker compose up --build test` or similar. The `test` container 
#    will wait a bit for the db & redis to start (if you have a `depends_on` or 
#    "sleep" in your command), then run these tests, connecting to real Redis.
#
# This ensures the sign-in flow, code verification, and JWT usage all function 
# properly with an actual Redis instance inside Docker Compose.
