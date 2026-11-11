package models

import "time"

type Company struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Symbol    string    `json:"symbol"`
	Exchange  string    `json:"exchange"`
	Country   string    `json:"country"`
	Sector    *string   `json:"sector"`
	Industry  *string   `json:"industry"`
	Currency  *string   `json:"currency"`
	Website   *string   `json:"website"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
