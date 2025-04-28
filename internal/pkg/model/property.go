package model

import "time"

type Property struct {
	Id         int64     `json:"id"`
	TimeStamp  time.Time `json:"timestamp"`
	Unit       string    `json:"unit_of_measurement"`
	Value      string    `json:"value"`
	Identifier string    `json:"identifier"`
	Slug       string    `json:"slug"`
}
type Properties []Property
