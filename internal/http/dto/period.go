package dto

import "time"

type PeriodInfo struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
	ParentID  *int64    `json:"parent_id"`
	Depth     int       `json:"depth"`
	Status    string    `json:"status"`
}

type PeriodsResponse struct {
	Items []PeriodInfo `json:"items"`
}
