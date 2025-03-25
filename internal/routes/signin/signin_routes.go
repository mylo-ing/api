package signin

import (
	"fiber-gorm-api/internal/handlers"
	redisclient "fiber-gorm-api/internal/redis"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

// RegisterRoutes sets up sign in routes under /signin
func RegisterRoutes(app *fiber.App) {
	signinGroup := app.Group("/signin", cors.New(cors.Config{
		AllowOrigins: "https://signin.mylocal.ing",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	// Initialize Redis
	redisclient.InitRedis("session")

	// Request a code by email
	signinGroup.Post("/request", handlers.RequestSignIn)

	// Verify the code to get a JWT
	signinGroup.Post("/verify", handlers.VerifySignIn)
}
