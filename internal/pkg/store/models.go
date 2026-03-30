// Package store defines the storage interface and the domain models shared
// across all database implementations (postgres, mssql, …).
// Consumers import only this package; implementation-specific packages live
// in sub-directories (store/postgres, store/mssql, …).
package store

import "time"

// Property is a normalised sensor reading stored in the database.
type Property struct {
	ID                int       `json:"id"`
	TimeStamp         time.Time `json:"time_stamp"`
	UnitOfMeasurement string    `json:"unit_of_measurement"`
	Value             string    `json:"value"`
	Identifier        string    `json:"identifier"`
	Slug              string    `json:"slug"`
}

// Amberprice is a single electricity price interval from the Amber API.
type Amberprice struct {
	ID          int       `json:"id"`
	PerKwh      float64   `json:"per_kwh"`
	SpotPerKwh  float64   `json:"spot_per_kwh"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Duration    int       `json:"duration"`
	Forecast    bool      `json:"forecast"`
	ChannelType string    `json:"channel_type"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Amberusage is a single energy usage interval from the Amber API.
type Amberusage struct {
	ID                int       `json:"id"`
	PerKwh            float64   `json:"per_kwh"`
	SpotPerKwh        float64   `json:"spot_per_kwh"`
	StartTime         time.Time `json:"start_time"`
	EndTime           time.Time `json:"end_time"`
	Duration          int       `json:"duration"`
	ChannelType       string    `json:"channel_type"`
	ChannelIdentifier string    `json:"channel_identifier"`
	Kwh               float64   `json:"kwh"`
	Quality           string    `json:"quality"`
	Cost              float64   `json:"cost"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// User is an application user account.
type User struct {
	ID           int       `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// RefreshToken is a persisted refresh-token record.
type RefreshToken struct {
	TokenHash string    `json:"token_hash"`
	UserID    int       `json:"user_id"`
	Username  string    `json:"username"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// StoreRefreshTokenParams are the arguments for persisting a new refresh token.
type StoreRefreshTokenParams struct {
	TokenHash string
	UserID    int
	Username  string
	ExpiresAt time.Time
}
