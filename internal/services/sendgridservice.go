package sendgridservice

import (
	"fmt"
	"log"
	"os"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

// SendCodeEmailFunc is a variable you can override in tests for mocking.
var SendCodeEmailFunc = defaultSendCodeEmail

// SendCodeEmail uses the official SendGrid client to send a sign-in code email.
func defaultSendCodeEmail(toEmail, code string) error {
	apiKey := os.Getenv("SENDGRID_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("SENDGRID_API_KEY not set, cannot send email")
	}

	fromAddress := os.Getenv("SENDGRID_FROM_ADDRESS")
	if fromAddress == "" {
		fromAddress = "no-reply@example.com" // fallback
		log.Printf("[WARN] SENDGRID_FROM_ADDRESS not set, using fallback '%s'\n", fromAddress)
	}

	from := mail.NewEmail("MyApp", fromAddress)
	to := mail.NewEmail("", toEmail)
	subject := "Your Sign-In Code"

	plainText := fmt.Sprintf("Your sign-in code is: %s\n\nUse this code to finish signing in.", code)
	htmlContent := fmt.Sprintf("<strong>Your sign-in code is: %s</strong><br>Use this code to finish signing in.", code)

	message := mail.NewSingleEmail(from, subject, to, plainText, htmlContent)

	client := sendgrid.NewSendClient(apiKey)
	response, err := client.Send(message)
	if err != nil {
		return fmt.Errorf("failed to send email via sendgrid: %w", err)
	}

	// For debugging/logging:
	if response.StatusCode >= 300 {
		log.Printf("[SendGrid] Non-success status code: %d\nBody: %s\n", response.StatusCode, response.Body)
		if response.StatusCode >= 400 && response.StatusCode < 500 {
			return fmt.Errorf("sendgrid returned client error (%d): %s", response.StatusCode, response.Body)
		} else if response.StatusCode >= 500 {
			return fmt.Errorf("sendgrid returned server error (%d): %s", response.StatusCode, response.Body)
		}
	} else {
		log.Printf("[SendGrid] Email sent successfully to %s, status: %d\n", toEmail, response.StatusCode)
	}

	return nil
}
