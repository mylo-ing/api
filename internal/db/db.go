package db

import (
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Connect(admin bool) *gorm.DB {
	var user string
	var password string
	if admin {
		user = os.Getenv("DB_ADMIN_USER")
		password = os.Getenv("DB_ADMIN_PASSWORD")
	} else {
		user = os.Getenv("DB_USER")
		password = os.Getenv("DB_PASSWORD")
	}
	host := os.Getenv("DB_HOST")
	dbname := os.Getenv("DB_NAME")
	port := os.Getenv("DB_PORT")
	sslmode := os.Getenv("DB_SSL_MODE")

	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		host, user, password, dbname, port, sslmode,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}

	return db
}
