package signup

import (
	"fiber-gorm-api/internal/db"
	"fiber-gorm-api/internal/handlers"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

// RegisterRoutes registers the signup group route with create-only for subscribers.
func RegisterRoutes(app *fiber.App) {
	signupGroup := app.Group("/signup", cors.New(cors.Config{
		AllowOrigins: "https://signup.mylocal.ing",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	subs := signupGroup.Group("/subscribers")

	// Initialize DB
	database := db.Connect(false)

	// Create only
	subs.Post("/", handlers.CreateSubscriber(database))
}
