package periods

import (
	"okrs/internal/core/domain"
	"okrs/internal/http/dto"
	v1 "okrs/internal/http/handlers/api/v1"
)

func newPeriodsResponse(views []domain.PeriodView) dto.PeriodsResponse {
	items := make([]dto.PeriodInfo, 0, len(views))
	for _, v := range views {
		items = append(items, v1.MapPeriodView(v))
	}
	return dto.PeriodsResponse{Items: items}
}
