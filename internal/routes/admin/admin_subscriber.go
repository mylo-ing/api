package admin

import (
	"fiber-gorm-api/internal/handlers"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// RegisterSubscriberRoutes registers the CRUD routes for subscribers under /admin/subscribers.
// NOTE: We don't separately register subscriber_types here as they are embedded in the subscriber routes.
func RegisterSubscriberRoutes(adminGroup fiber.Router, db *gorm.DB) {
	subs := adminGroup.Group("/subscribers")

	// Create
	subs.Post("/", handlers.CreateSubscriber(db))

	// Read all
	subs.Get("/", handlers.GetAllSubscribers(db))

	// Read single
	subs.Get("/:id", handlers.GetSubscriber(db))

	// Update
	subs.Put("/:id", handlers.UpdateSubscriber(db))

	// Delete
	subs.Delete("/:id", handlers.DeleteSubscriber(db))
}
