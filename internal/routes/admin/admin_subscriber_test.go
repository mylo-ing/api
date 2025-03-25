package admin

import (
	"encoding/json"
	"fiber-gorm-api/internal/db"
	"fiber-gorm-api/internal/middleware"
	"fiber-gorm-api/internal/models"
	redisclient "fiber-gorm-api/internal/redis"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// Example Admin test that requires JWT.
// This file merges your existing subscriber tests with token checks.
// It assumes you have:
//   1) middleware.RequireJWT
//   2) middleware.GenerateJWT
//   3) redisclient.InitRedis / redisclient.SetValue
// in your codebase.
//
// If your "RegisterSubscriberRoutes" is actually behind JWT in your production code
// (e.g., in "RegisterAdminRoutes" with "app.Use(middleware.RequireJWT)"), you must
// replicate that arrangement here.
//
// For demonstration, we do it inline: app.Use(middleware.RequireJWT).
// If you want it exactly as in production, just ensure the route group has the RequireJWT
// middleware. The test approach is the same: supply a valid token header or expect 401.

func TestAdminSubscriberRoutes(t *testing.T) {
	// 1) Connect to real DB or ephemeral DB.
	//    `db.Connect(true)` presumably returns a GORM DB connected to your test DB or in-memory DB.
	database := db.Connect(true)

	// 2) If you haven't already initialized Redis, do it once:
	redisclient.InitRedis("session")

	// 3) Create a fresh Fiber app with your admin routes.
	//    We also inject the RequireJWT middleware for all these endpoints.
	app := fiber.New()
	app.Use(middleware.RequireJWT) // <--- enforce JWT
	RegisterSubscriberRoutes(app, database)

	// 4) Create a helper function to build requests with valid token or intentionally missing/invalid token
	getRequestWithToken := func(method, url string, body io.Reader, validToken bool) (*http.Request, error) {
		// If the caller provided nil, interpret it as an empty body
		if body == nil {
			body = strings.NewReader("")
		}

		req := httptest.NewRequest(method, url, body)
		req.Header.Set("Content-Type", "application/json")

		// If we want a valid token, create a session in Redis + generate a JWT
		if validToken {
			sessionID := "adminRouteTestSession"
			userData := `{"email":"admin@example.com"}`
			redisKey := "session:" + sessionID

			if err := redisclient.SetValue(redisKey, userData, 0); err != nil {
				return nil, fmt.Errorf("failed to store session in redis: %w", err)
			}
			token, err := middleware.GenerateJWT(sessionID)
			if err != nil {
				return nil, fmt.Errorf("failed to generate token: %w", err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
		}

		return req, nil
	}

	////////////////////////////////////////////////////////////////
	// FIRST, TEST NO TOKEN => 401
	////////////////////////////////////////////////////////////////
	t.Run("No Token => 401", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/subscribers", nil)
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("Expected 401, got %d", resp.StatusCode)
		}
	})

	////////////////////////////////////////////////////////////////
	// NOW RE-RUN YOUR EXISTING TESTS, BUT WITH A VALID TOKEN
	////////////////////////////////////////////////////////////////
	t.Run("CreateSubscriber - EmptyBody", func(t *testing.T) {
		req, err := getRequestWithToken("POST", "/subscribers", strings.NewReader(""), true)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400 for empty body, got %d", resp.StatusCode)
		}
	})

	t.Run("CreateSubscriber - Missing Email", func(t *testing.T) {
		payload := `{"name": "John Doe"}`
		req, err := getRequestWithToken("POST", "/subscribers", strings.NewReader(payload), true)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400 for missing email, got %d", resp.StatusCode)
		}
	})

	t.Run("CreateSubscriber - Missing Name", func(t *testing.T) {
		payload := `{"email": "john@example.com"}`
		req, err := getRequestWithToken("POST", "/subscribers", strings.NewReader(payload), true)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400 for missing name, got %d", resp.StatusCode)
		}
	})

	t.Run("CreateSubscriber - Invalid Email", func(t *testing.T) {
		payload := `{"email": "notanemail", "name": "John" }`
		req, err := getRequestWithToken("POST", "/subscribers", strings.NewReader(payload), true)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400 for invalid email, got %d", resp.StatusCode)
		}
	})

	t.Run("CreateSubscriber - Valid Subscriber with Types", func(t *testing.T) {
		payload := `{
			"email": "john@example.com",
			"name": "John",
			"subscriber_types": [
				{"name": "shopper"},
				{"name": "donor"}
			]
		}`
		req, err := getRequestWithToken("POST", "/subscribers", strings.NewReader(payload), true)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		resp, err := app.Test(req, -1) // no timeout
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected 201, got %d", resp.StatusCode)
		}

		var created models.Subscriber
		json.NewDecoder(resp.Body).Decode(&created)

		if created.ID == 0 {
			t.Errorf("Expected subscriber to have an ID after creation")
		}
		if len(created.SubscriberTypes) != 2 {
			t.Errorf("Expected 2 subscriber_types, got %d", len(created.SubscriberTypes))
		}
	})

	t.Run("GetSubscriber - Not Found", func(t *testing.T) {
		req, err := getRequestWithToken("GET", "/subscribers/999", nil, true)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404 for non-existent subscriber, got %d", resp.StatusCode)
		}
	})

	t.Run("GetSubscriber - Success", func(t *testing.T) {
		// Create a subscriber first
		s := models.Subscriber{Email: "test-get@example.com", Name: "Tester"}
		database.Create(&s)

		path := fmt.Sprintf("/subscribers/%d", s.ID)
		req, err := getRequestWithToken("GET", path, nil, true)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("UpdateSubscriber - Not Found", func(t *testing.T) {
		payload := `{"email": "updated@example.com", "name": "Updater"}`
		req, err := getRequestWithToken("PUT", "/subscribers/999", strings.NewReader(payload), true)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404 when updating non-existent subscriber, got %d", resp.StatusCode)
		}
	})

	t.Run("UpdateSubscriber - Invalid Email", func(t *testing.T) {
		// Create a subscriber first
		s := models.Subscriber{Email: "test-update@example.com", Name: "Tester"}
		database.Create(&s)

		payload := `{"email": "invalidEmail", "name": "Updated Tester"}`
		path := fmt.Sprintf("/subscribers/%d", s.ID)
		req, err := getRequestWithToken("PUT", path, strings.NewReader(payload), true)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400 for invalid email, got %d", resp.StatusCode)
		}
	})

	t.Run("UpdateSubscriber - Success", func(t *testing.T) {
		// Create a subscriber first
		s := models.Subscriber{Email: "old-email@example.com", Name: "Old Name"}
		database.Create(&s)

		payload := `{"email": "new-email@example.com", "name": "New Name", "subscriber_types":[{"name":"developer"}]}`
		path := fmt.Sprintf("/subscribers/%d", s.ID)
		req, err := getRequestWithToken("PUT", path, strings.NewReader(payload), true)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}

		// Check the updated record
		var updated models.Subscriber
		database.Preload("SubscriberTypes").First(&updated, s.ID)
		if updated.Email != "new-email@example.com" {
			t.Errorf("Email not updated properly, got %s", updated.Email)
		}
		if updated.Name != "New Name" {
			t.Errorf("Name not updated properly, got %s", updated.Name)
		}
		if len(updated.SubscriberTypes) != 1 {
			t.Errorf("Expected 1 subscriber_type, got %d", len(updated.SubscriberTypes))
		}
	})

	t.Run("DeleteSubscriber - Not Found", func(t *testing.T) {
		req, err := getRequestWithToken("DELETE", "/subscribers/999", nil, true)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404 when deleting non-existent subscriber, got %d", resp.StatusCode)
		}
	})

	t.Run("DeleteSubscriber - Success", func(t *testing.T) {
		// Create a subscriber first
		s := models.Subscriber{Email: "delete-me@example.com", Name: "ToDelete"}
		database.Create(&s)

		path := fmt.Sprintf("/subscribers/%d", s.ID)
		req, err := getRequestWithToken("DELETE", path, nil, true)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("Expected 204, got %d", resp.StatusCode)
		}

		// Ensure it's actually deleted
		var check models.Subscriber
		result := database.First(&check, s.ID)
		if result.Error == nil {
			t.Errorf("Expected subscriber to be deleted, but it still exists.")
		}
	})
}
