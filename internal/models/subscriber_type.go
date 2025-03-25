package models

import "time"

// SubscriberType references the Postgres ENUM column, e.g. 'shopper', 'business', etc.
// This table references a single Subscriber record (one subscriber -> many subscriber_types).
type SubscriberType struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	SubscriberID uint      `json:"subscriber_id"`
	Name         string    `gorm:"type:subscriber_type;not null" json:"name"` // references the custom ENUM
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
