package handlers

import (
	"fiber-gorm-api/internal/models"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// A more robust email regex to ensure an address-like format.
// (Though there's no perfect regex for all valid emails, this is a decent approach.)
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// validateSubscriberFields performs stricter checks on email and name
func validateSubscriberFields(sub *models.Subscriber) error {
	// Email must not be empty, must contain '@', must match our robust pattern
	if sub.Email == "" || !emailRegex.MatchString(sub.Email) {
		return fmt.Errorf("invalid or missing email")
	}

	// Name must be non-empty
	if strings.TrimSpace(sub.Name) == "" {
		return fmt.Errorf("missing name")
	}
	return nil
}

// CreateSubscriber godoc
// @Summary      Create a new subscriber
// @Description  Creates a new subscriber record, optionally with multiple subscriber_types. Validates email & name.
// @Tags         subscribers
// @Accept       json
// @Produce      json
// @Param        subscriber  body      models.Subscriber  true  "Subscriber info (with subscriber_types optional)"
// @Success      201         {object}  models.Subscriber
// @Failure      400         {string}  string
// @Failure      500         {string}  string
// @Router       /admin/subscribers [post]
// @Router       /signup/subscribers [post]
func CreateSubscriber(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var subscriber models.Subscriber
		if err := c.BodyParser(&subscriber); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Unable to parse request body"})
		}

		// Validate email & name
		if err := validateSubscriberFields(&subscriber); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}

		err := db.Create(&subscriber).Error
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": fmt.Sprintf("Could not create subscriber: %v", err),
			})
		}

		// Return with joined subscriber_types
		if err := db.Preload("SubscriberTypes").First(&subscriber, subscriber.ID).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to load created subscriber with subscriber_types",
			})
		}
		return c.Status(fiber.StatusCreated).JSON(subscriber)
	}
}

// GetAllSubscribers godoc
// @Summary      Get all subscribers
// @Description  Returns a list of all subscribers, including their subscriber_types
// @Tags         subscribers
// @Produce      json
// @Success      200  {array}   models.Subscriber
// @Failure      500  {string}  string
// @Router       /admin/subscribers [get]
func GetAllSubscribers(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var subscribers []models.Subscriber
		if err := db.Preload("SubscriberTypes").Find(&subscribers).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Could not retrieve subscribers",
			})
		}
		return c.JSON(subscribers)
	}
}

// GetSubscriber godoc
// @Summary      Get a single subscriber
// @Description  Gets subscriber by id, including all subscriber_types
// @Tags         subscribers
// @Produce      json
// @Param        id   path      int true "Subscriber ID"
// @Success      200  {object}  models.Subscriber
// @Failure      400  {string}  string
// @Failure      404  {string}  string
// @Router       /admin/subscribers/{id} [get]
func GetSubscriber(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		idParam := c.Params("id")
		id, err := strconv.Atoi(idParam)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid subscriber ID"})
		}

		var subscriber models.Subscriber
		if err := db.Preload("SubscriberTypes").First(&subscriber, id).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Subscriber not found"})
		}
		return c.JSON(subscriber)
	}
}

// UpdateSubscriber godoc
// @Summary      Update a subscriber
// @Description  Updates subscriber by id. If subscriber_types are provided, it overwrites them. Validates email & name.
// @Tags         subscribers
// @Accept       json
// @Produce      json
// @Param        id   path      int true "Subscriber ID"
// @Param        subscriber  body      models.Subscriber  true  "Subscriber info (subscriber_types optional)"
// @Success      200  {object}  models.Subscriber
// @Failure      400  {string}  string
// @Failure      404  {string}  string
// @Failure      500  {string}  string
// @Router       /admin/subscribers/{id} [put]
func UpdateSubscriber(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		idParam := c.Params("id")
		id, convErr := strconv.Atoi(idParam)
		if convErr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid subscriber ID"})
		}

		// Get existing subscriber
		var existing models.Subscriber
		if err := db.Preload("SubscriberTypes").First(&existing, id).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Subscriber not found"})
		}

		// Parse the incoming updates
		var updates models.Subscriber
		if err := c.BodyParser(&updates); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Unable to parse request body"})
		}

		// Validate email & name
		if err := validateSubscriberFields(&updates); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}

		// Update basic fields
		existing.Email = updates.Email
		existing.Name = updates.Name

		// If subscriber_types are present, overwrite
		if updates.SubscriberTypes != nil && len(updates.SubscriberTypes) > 0 {
			// remove all existing subscriber_types
			if err := db.Where("subscriber_id = ?", existing.ID).Delete(&models.SubscriberType{}).Error; err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "Could not clear old subscriber_types",
				})
			}
			// create new subscriber_types referencing existing.ID
			for i := range updates.SubscriberTypes {
				updates.SubscriberTypes[i].SubscriberID = existing.ID
			}
			if err := db.Create(&updates.SubscriberTypes).Error; err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "Could not create updated subscriber_types",
				})
			}
		} else if updates.SubscriberTypes != nil && len(updates.SubscriberTypes) == 0 {
			// If an empty slice was explicitly passed
			if err := db.Where("subscriber_id = ?", existing.ID).Delete(&models.SubscriberType{}).Error; err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "Could not remove subscriber_types",
				})
			}
		}

		// Save subscriber base fields
		if err := db.Save(&existing).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Could not update subscriber",
			})
		}

		// Return with joined subscriber_types
		if err := db.Preload("SubscriberTypes").First(&existing, existing.ID).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to fetch updated subscriber",
			})
		}

		return c.JSON(existing)
	}
}

// DeleteSubscriber godoc
// @Summary      Delete a subscriber
// @Description  Deletes subscriber by id (and associated subscriber_types).
// @Tags         subscribers
// @Param        id   path      int true "Subscriber ID"
// @Success      204  {string}  string
// @Failure      400  {string}  string
// @Failure      404  {string}  string
// @Failure      500  {string}  string
// @Router       /admin/subscribers/{id} [delete]
func DeleteSubscriber(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		idParam := c.Params("id")
		id, convErr := strconv.Atoi(idParam)
		if convErr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid subscriber ID"})
		}

		var subscriber models.Subscriber
		if err := db.First(&subscriber, id).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Subscriber not found"})
		}

		// Remove subscriber_types first (if not using a cascade constraint).
		if err := db.Where("subscriber_id = ?", subscriber.ID).Delete(&models.SubscriberType{}).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Could not delete subscriber_types",
			})
		}

		if err := db.Delete(&subscriber).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Could not delete subscriber",
			})
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
}
