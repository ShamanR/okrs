package common

import (
	"fmt"
	"time"
)

func FormatAbsoluteTime(value time.Time, zone *time.Location) string {
	if value.IsZero() {
		return ""
	}
	if zone != nil {
		value = value.In(zone)
	}
	return value.Format("2006-01-02 15:04")
}

func RelativeTime(value time.Time, now time.Time) string {
	if value.IsZero() {
		return "нет данных"
	}
	if now.IsZero() {
		now = time.Now()
	}
	if value.After(now) {
		value = now
	}
	diff := now.Sub(value)
	switch {
	case diff < time.Minute:
		return "только что"
	case diff < time.Hour:
		minutes := int(diff.Minutes())
		return fmt.Sprintf("%d %s назад", minutes, pluralRu(minutes, "минута", "минуты", "минут"))
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		return fmt.Sprintf("%d %s назад", hours, pluralRu(hours, "час", "часа", "часов"))
	case diff < 30*24*time.Hour:
		days := int(diff.Hours() / 24)
		return fmt.Sprintf("%d %s назад", days, pluralRu(days, "день", "дня", "дней"))
	case diff < 365*24*time.Hour:
		months := int(diff.Hours() / (24 * 30))
		if months == 0 {
			months = 1
		}
		return fmt.Sprintf("%d %s назад", months, pluralRu(months, "месяц", "месяца", "месяцев"))
	default:
		years := int(diff.Hours() / (24 * 365))
		if years == 0 {
			years = 1
		}
		return fmt.Sprintf("%d %s назад", years, pluralRu(years, "год", "года", "лет"))
	}
}

func pluralRu(count int, one, few, many string) string {
	if count%100 >= 11 && count%100 <= 14 {
		return many
	}
	switch count % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	default:
		return many
	}
}
