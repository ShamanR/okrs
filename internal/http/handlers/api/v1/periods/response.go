package periods

import (
	"okrs/internal/domain"
	"okrs/internal/http/dto"
	v1 "okrs/internal/http/handlers/api/v1"
)

func newPeriodsResponse(periods []domain.Period) dto.PeriodsResponse {
	items := make([]dto.PeriodInfo, 0, len(periods))
	for _, period := range periods {
		items = append(items, v1.MapPeriodInfo(period))
	}
	return dto.PeriodsResponse{Items: items}
}
