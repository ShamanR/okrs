package common

import (
	"testing"

	"okrs/internal/core/domain"
)

func TestValidTeamType(t *testing.T) {
	valid := []domain.TeamType{
		domain.TeamTypeDepartment,
		domain.TeamTypeCluster,
		domain.TeamTypeUnit,
		domain.TeamTypeGroup,
		domain.TeamTypeTeam,
		domain.TeamTypeSquad,
		domain.TeamTypeEmployee,
	}
	for _, tt := range valid {
		if !ValidTeamType(tt) {
			t.Errorf("ValidTeamType(%q) = false, want true", tt)
		}
	}

	invalid := []domain.TeamType{"", "guild", "division", "Team"}
	for _, tt := range invalid {
		if ValidTeamType(tt) {
			t.Errorf("ValidTeamType(%q) = true, want false", tt)
		}
	}
}

func TestTeamTypeLabel(t *testing.T) {
	cases := map[domain.TeamType]string{
		domain.TeamTypeDepartment: "Департамент",
		domain.TeamTypeCluster:    "Кластер",
		domain.TeamTypeUnit:       "Юнит",
		domain.TeamTypeGroup:      "Группа",
		domain.TeamTypeTeam:       "Команда",
		domain.TeamTypeSquad:      "Сквад",
		domain.TeamTypeEmployee:   "Сотрудник",
		"unknown":                 "Команда", // fallback
	}
	for tt, want := range cases {
		if got := TeamTypeLabel(tt); got != want {
			t.Errorf("TeamTypeLabel(%q) = %q, want %q", tt, got, want)
		}
	}
}
