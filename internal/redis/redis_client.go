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
func InitRedis(usage string) {
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		log.Fatalf("You must specificy a Redis host environment variable.")
	}

	var dbType string
	if usage == "session" {
		dbType = "REDIS_SESSION_DB"
	} else {
		dbType = "REDIS_ENTITY_DB"
	}
	dbStr := os.Getenv(dbType)

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
