package admin

import (
	"fiber-gorm-api/internal/db"
	"fiber-gorm-api/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

// RegisterAdminRoutes configures the admin group, applying CORS for admin.mylocal.ing
// and registers all admin route files (subscribers).
func RegisterAdminRoutes(app *fiber.App) {
	adminGroup := app.Group("/admin", cors.New(cors.Config{
		AllowOrigins: "https://admin.mylocal.ing",
		AllowHeaders: "Origin, Content-Type, Accept",
	}),
		middleware.RequireJWT, // <--- Enforce JWT for all admin routes
	)

	// Initialize DB
	database := db.Connect(true)

	// Subscribers CRUD
	RegisterSubscriberRoutes(adminGroup, database)
}
