package models

import "time"

// Subscriber represents a single subscriber record.
// A subscriber can have MANY subscriber_types records referencing it.
type Subscriber struct {
	ID               uint             `gorm:"primaryKey" json:"id"`
	Email            string           `gorm:"type:varchar(255);not null" json:"email"`
	Name             string           `gorm:"type:varchar(255)" json:"name"`
	SubscriberTypes  []SubscriberType `gorm:"foreignKey:SubscriberID" json:"subscriber_types,omitempty"`
	CreatedAt        time.Time        `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time        `gorm:"autoUpdateTime" json:"updated_at"`
}
