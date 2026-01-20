package store

import (
	"time"

	"okrs/internal/domain"
)

func CurrentQuarter(now time.Time) domain.Quarter {
	month := int(now.Month())
	quarter := ((month - 1) / 3) + 1
	return domain.Quarter{Year: now.Year(), Quarter: quarter}
}
