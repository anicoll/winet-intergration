package model

import "time"

type AmberPrice struct {
	ID          int       `json:"id"`
	PerKwh      float32   `json:"perKwh"`
	SpotPerKwh  float32   `json:"spotPerKwh"`
	EndTime     time.Time `json:"end_time"`
	StartTime   time.Time `json:"start_time"`
	Duration    int       `json:"duration"`     // in minutes
	Forecast    bool      `json:"forecast"`     // indicates if the price is a forecast price or not.
	ChannelType string    `json:"channel_type"` // indicates if the price is feedin or general.
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AmberPrices []AmberPrice
