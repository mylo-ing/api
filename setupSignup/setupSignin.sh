# Below are only the NEW or CHANGED files required to add a "sign in" flow that:
#  1) Receives an email and generates a 6-digit numeric code
#  2) Sends that code via SendGrid (placeholder shown)
#  3) Stores the code in Redis
#  4) Verifies the code to produce a JWT token
#  5) Stores user session and profile in Redis
#  6) Requires JWT for all admin routes
#
# Everything else (the existing routes, folder structure, etc.) remains the same.
# Just merge these files/changes with your current codebase. 
#
# *IMPORTANT* 
# - In real usage, add your own SendGrid API logic and environment variables.
# - Ensure your Redis and JWT secret are configured properly (through env variables or secrets).
#
# (No minimal examples—this is a more complete approach. Adjust as needed.)

================================================================================
CHANGED FILE: go.mod
================================================================================
module fiber-gorm-api

go 1.20

require (
	github.com/gofiber/fiber/v2 v2.42.0
	github.com/gofiber/swagger v1.0.8
	gorm.io/driver/postgres v1.5.2
	gorm.io/gorm v1.24.5
	
	# JWT and Redis additions:
	github.com/golang-jwt/jwt/v4 v4.4.3
	github.com/redis/go-redis/v9 v9.0.0
)

require (
	github.com/swaggo/files v1.1.3 // indirect
	github.com/swaggo/swag v1.8.13 // indirect
)


================================================================================
NEW FILE: internal/redis/redis_client.go
================================================================================
package redisclient

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

var Rdb *redis.Client
var Ctx = context.Background()

// InitRedis initializes the Redis client from environment variables
func InitRedis() {
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "localhost:6379"
	}

	dbStr := os.Getenv("REDIS_DB")
	if dbStr == "" {
		dbStr = "0"
	}
	dbNum, err := strconv.Atoi(dbStr)
	if err != nil {
		dbNum = 0
	}

	Rdb = redis.NewClient(&redis.Options{
		Addr:     host,
		Password: os.Getenv("REDIS_PASSWORD"), // set via environment secrets if needed
		DB:       dbNum,
	})

	// test connection
	_, err = Rdb.Ping(Ctx).Result()
	if err != nil {
		log.Fatalf("Could not connect to Redis: %v", err)
	}
	log.Println("Connected to Redis on", host)
}

// SetValue stores a string value in Redis with an expiration
func SetValue(key, value string, expiration time.Duration) error {
	return Rdb.Set(Ctx, key, value, expiration).Err()
}

// GetValue retrieves a string value from Redis
func GetValue(key string) (string, error) {
	return Rdb.Get(Ctx, key).Result()
}

// DeleteKey removes a key from Redis
func DeleteKey(key string) error {
	return Rdb.Del(Ctx, key).Err()
}


================================================================================
NEW FILE: internal/middleware/jwt_middleware.go
================================================================================
package middleware

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"fiber-gorm-api/internal/redisclient"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
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

	// parse and validate token
	claims := jwt.MapClaims{}
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "devsecret" // fallback for local dev
	}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid or expired token"})
	}

	// Optionally, we can check Redis for a user session
	sessionKey, ok := claims["session_key"].(string)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Session key missing in token"})
	}

	// Check session in redis
	sessionVal, err := redisclient.GetValue("session:" + sessionKey)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Session not found or expired"})
	}
	if sessionVal == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Session invalid or not found"})
	}

	// everything is good, let next handle
	return c.Next()
}

// GenerateJWT creates a new JWT with the given session key
func GenerateJWT(sessionKey string) (string, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "devsecret" // fallback for local dev
	}

	claims := jwt.MapClaims{
		"session_key": sessionKey,
		"exp":         jwt.NewNumericDate(jwt.TimeFunc().AddDate(0, 0, 1)), // 1 day expiration
		"iat":         jwt.NewNumericDate(jwt.TimeFunc()),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", err
	}
	return ss, nil
}


================================================================================
NEW FILE: internal/routes/signin/signin.go
================================================================================
package signin

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"time"

	"fiber-gorm-api/internal/redisclient"
	"fiber-gorm-api/internal/routes/signin/sendgridservice"
	"fiber-gorm-api/internal/middleware"

	"github.com/gofiber/fiber/v2"
)

// signInCodeKey is a helper to form the Redis key for storing a code
func signInCodeKey(email string) string {
	return "signin_code:" + email
}

// sessionKey is used for storing user session data
func sessionKey(email string) string {
	return "session:" + email
}

// Generate a random 6-digit numeric code
func generateSixDigitCode() string {
	// This is one approach: generate random bytes, parse mod 1,000,000
	// Then zero-pad to 6 digits. 
	var b [3]byte
	_, err := rand.Read(b[:])
	if err != nil {
		log.Println("Failed to generate random bytes, falling back to time-based code.")
		now := time.Now().UnixNano()
		return fmt.Sprintf("%06d", now%1000000)
	}
	num := (int(b[0])<<16 | int(b[1])<<8 | int(b[2])) % 1000000
	return fmt.Sprintf("%06d", num)
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
func requestSignIn(c *fiber.Ctx) error {
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

	// send code via sendgrid (dummy function here)
	if err := sendgridservice.SendCodeEmail(req.Email, code); err != nil {
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
func verifySignIn(c *fiber.Ctx) error {
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

	// compare
	if storedCode != req.Code {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid code"})
	}

	// Remove the code from redis (single-use)
	_ = redisclient.DeleteKey(signInCodeKey(req.Email))

	// Create a user session (in a real app, we'd store user roles, etc.)
	// We'll store a random session ID in redis, referencing minimal user profile
	sessionID := randomToken(16)
	userProfile := fmt.Sprintf(`{"email":"%s"}`, req.Email)
	if err := redisclient.SetValue("session:"+sessionID, userProfile, 24*time.Hour); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not store session"})
	}

	// Generate JWT that references this session
	token, err := middleware.GenerateJWT(sessionID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not create token"})
	}

	return c.JSON(fiber.Map{
		"token": token,
	})
}

// randomToken returns a URL-safe random string
func randomToken(length int) string {
	raw := make([]byte, length)
	_, _ = rand.Read(raw)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// RegisterRoutes sets up sign in routes under /signin
func RegisterRoutes(app *fiber.App) {
	signinGroup := app.Group("/signin")

	signinGroup.Post("/request", requestSignIn)
	signinGroup.Post("/verify", verifySignIn)
}


================================================================================
NEW FILE: internal/routes/signin/sendgridservice/sendgridservice.go
================================================================================
package sendgridservice

import (
	"fmt"
	"log"
	"os"
)

// SendCodeEmail is a placeholder function that would use the SendGrid API to send an email.
// In your actual code, you'd import the real "github.com/sendgrid/sendgrid-go" etc. 
// Then you'd replace this stub with real usage.
func SendCodeEmail(toEmail, code string) error {
	// Pseudocode: Using environment variable SENDGRID_API_KEY
	apiKey := os.Getenv("SENDGRID_API_KEY")
	if apiKey == "" {
		log.Println("No SENDGRID_API_KEY set, skipping real email send. (Placeholder only)")
	}

	log.Printf("[DEBUG] Sending sign-in code '%s' to '%s' using sendgrid...\n", code, toEmail)
	// Real usage might be something like:
	// sgClient := sendgrid.NewSendClient(apiKey)
	// message := mail.NewSingleEmail(from, subject, to, plaintextContent, htmlContent)
	// response, err := sgClient.Send(message)
	// ...
	// For now, we just print to logs
	fmt.Printf("Successfully 'sent' code %s to email %s.\n", code, toEmail)

	return nil
}


================================================================================
CHANGED FILE: main.go (adding Redis init & sign-in routes)
================================================================================
package main

import (
	"log"
	"os"

	_ "fiber-gorm-api/docs" // swagger docs

	"fiber-gorm-api/internal/db"
	"fiber-gorm-api/internal/redisclient"
	"fiber-gorm-api/internal/routes/admin"
	"fiber-gorm-api/internal/routes/signin"
	"fiber-gorm-api/internal/routes/signup"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	swagger "github.com/gofiber/swagger"
)

// @title           Fiber GORM API
// @version         1.0
// @description     This is an API using Fiber + GORM (Postgres) with separate admin, signup, and sign-in routes.
//
// @contact.name    API Support
// @contact.url     http://admin.mylocal.ing
// @contact.email   support@admin.mylocal.ing
//
// @license.name    MIT
// @license.url     https://opensource.org/licenses/MIT
//
// @BasePath  /
// @schemes http https

func main() {
	// Collect DB connection parameters from environment
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	if dbHost == "" || dbPort == "" || dbUser == "" || dbName == "" {
		log.Println("WARNING: Some required DB env variables are missing (DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME).")
	}

	// Initialize PostgreSQL
	database, err := db.InitDB(dbHost, dbPort, dbUser, dbPassword, dbName)
	if err != nil {
		log.Fatalf("Could not connect to the database: %v", err)
	}

	// Initialize Redis
	redisclient.InitRedis()

	// Fiber app
	app := fiber.New()

	// Logger middleware
	app.Use(logger.New())

	// Swagger route
	app.Get("/swagger/*", swagger.HandlerDefault)

	// Register sign-in routes
	signin.RegisterRoutes(app)

	// Register signup routes
	signup.RegisterRoutes(app, database)

	// Register admin routes (JWT required for admin)
	admin.RegisterAdminRoutes(app, database)

	// Start
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "3000"
	}
	log.Printf("Starting server on :%s", port)
	log.Fatal(app.Listen(":" + port))
}


================================================================================
CHANGED FILE: internal/routes/admin/admin_routes.go 
(added the JWT requirement)
================================================================================
package admin

import (
	"fiber-gorm-api/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"gorm.io/gorm"
)

// RegisterAdminRoutes configures the admin group, applying CORS for admin.mylocal.ing
// AND requiring a JWT token for access
func RegisterAdminRoutes(app *fiber.App, db *gorm.DB) {
	adminGroup := app.Group("/admin",
		cors.New(cors.Config{
			AllowOrigins: "https://admin.mylocal.ing",
			AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		}),
		middleware.RequireJWT, // <--- Enforce JWT for all admin routes
	)

	// Subscribers CRUD
	RegisterSubscriberRoutes(adminGroup, db)
}


# End of new/changed files

