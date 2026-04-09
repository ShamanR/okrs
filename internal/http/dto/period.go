package dto

import "time"

type PeriodInfo struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
	SortOrder int       `json:"sort_order"`
}

type PeriodsResponse struct {
	Items []PeriodInfo `json:"items"`
}
