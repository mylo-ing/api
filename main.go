package main

import (
	"log"
	"os"

	_ "fiber-gorm-api/docs" // swagger docs

	"fiber-gorm-api/internal/routes/admin"
	"fiber-gorm-api/internal/routes/signin"
	"fiber-gorm-api/internal/routes/signup"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	swagger "github.com/gofiber/swagger"
)

// @title           myLocal Headless API
// @version         1.0
// @description     The myLocal headless API is built in Go with Fiber and GORM.
// @contact.name    myLo API Support
// @contact.url     https://github.com/mylo-ing/api/issues
// @contact.email   info@mylo.ing
// @license.name    AGPLv3
// @host            localhost:3517
// @BasePath        /

func main() {
	// Fiber app
	app := fiber.New()

	// Logger middleware
	app.Use(logger.New())

	// Swagger route
	app.Get("/swagger/*", swagger.HandlerDefault)

	// Register sign-in routes
	signin.RegisterRoutes(app)

	// Register admin routes
	admin.RegisterAdminRoutes(app)

	// Register signup routes
	signup.RegisterRoutes(app)

	// Start
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "3000"
	}
	log.Printf("Starting server on :%s", port)
	log.Fatal(app.Listen(":" + port))
}
