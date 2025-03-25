package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"time"

	"fiber-gorm-api/internal/middleware"
	redisclient "fiber-gorm-api/internal/redis"
	sendgridservice "fiber-gorm-api/internal/services"

	"github.com/gofiber/fiber/v2"
)

// Helper to form the Redis key for storing a sign-in code for the given email
func signInCodeKey(email string) string {
	return "signin_code:" + email
}

// requestSignIn godoc
// @Summary      Request Sign In
// @Description  Takes an email, generates a 6-digit code, stores in Redis, sends via SendGrid
// @Tags         signin
// @Accept       json
// @Produce      json
// @Param        body  body      map[string]string  true  "e.g. { \"email\": \"user@example.com\" }"
// @Success      200   {object}  map[string]string  "Code sent"
// @Failure      400   {string}  string
// @Router       /signin/request [post]
func RequestSignIn(c *fiber.Ctx) error {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}
	if req.Email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Missing email"})
	}

	code := generateSixDigitCode()

	// store code in redis with 5 minute expiration
	if err := redisclient.SetValue(signInCodeKey(req.Email), code, 5*time.Minute); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Unable to store code in redis"})
	}

	// send code via sendgrid (stub function in 'sendgridservice')
	if err := sendgridservice.SendCodeEmailFunc(req.Email, code); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send email"})
	}

	return c.JSON(fiber.Map{
		"message": "A sign-in code has been emailed to you.",
	})
}

// verifySignIn godoc
// @Summary      Verify Sign In Code
// @Description  Takes an email and 6-digit code. If valid, generate JWT & store session in redis
// @Tags         signin
// @Accept       json
// @Produce      json
// @Param        body  body  map[string]string  true  "e.g. { \"email\": \"user@example.com\", \"code\": \"123456\" }"
// @Success      200   {object}  map[string]string  "JWT returned"
// @Failure      400   {string}  string
// @Router       /signin/verify [post]
func VerifySignIn(c *fiber.Ctx) error {
	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}
	if req.Email == "" || req.Code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Missing email or code"})
	}

	// retrieve code from redis
	storedCode, err := redisclient.GetValue(signInCodeKey(req.Email))
	if err != nil || storedCode == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "No sign-in code found or code expired"})
	}

	if storedCode != req.Code {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid code"})
	}

	// Remove the code from redis (single-use)
	_ = redisclient.DeleteKey(signInCodeKey(req.Email))

	// Create user session (store minimal user profile in Redis)
	sessionID := randomToken(16)
	userProfile := fmt.Sprintf(`{"email":"%s"}`, req.Email)
	if err := redisclient.SetValue("session:"+sessionID, userProfile, 24*time.Hour); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not store session"})
	}

	// Generate JWT referencing this session
	token, err := middleware.GenerateJWT(sessionID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not create token"})
	}

	return c.JSON(fiber.Map{
		"token": token,
	})
}

// Generate a random 6-digit numeric code
func generateSixDigitCode() string {
	var b [3]byte
	_, err := rand.Read(b[:])
	if err != nil {
		log.Println("Failed to generate random bytes, fallback to time-based code.")
		now := time.Now().UnixNano()
		return fmt.Sprintf("%06d", now%1000000)
	}
	num := (int(b[0])<<16 | int(b[1])<<8 | int(b[2])) % 1000000
	return fmt.Sprintf("%06d", num)
}

// randomToken returns a URL-safe random string
func randomToken(length int) string {
	raw := make([]byte, length)
	_, _ = rand.Read(raw)
	return base64.RawURLEncoding.EncodeToString(raw)
}
