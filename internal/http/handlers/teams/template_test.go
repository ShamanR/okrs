package teams

import (
	"os"
	"strings"
	"testing"
)

func TestTeamOKRTemplateHasDataAttributes(t *testing.T) {
	data, err := os.ReadFile("../../templates/team_okr.html")
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	content := string(data)
	required := []string{
		"data-page=\"team-okr\"",
		"data-team-id",
		"data-period-name",
		"data-okr-breadcrumbs",
		"data-okr-actions",
		"data-okr-goals",
	}
	for _, token := range required {
		if !strings.Contains(content, token) {
			t.Fatalf("expected template to include %q", token)
		}
	}
	forbidden := []string{
		"Сводка периода",
		"Цели периода",
		"data-okr-summary",
	}
	for _, token := range forbidden {
		if strings.Contains(content, token) {
			t.Fatalf("expected template not to include %q", token)
		}
	}
}

func TestTeamManageTemplateHasDeletedTeamsSection(t *testing.T) {
	data, err := os.ReadFile("../../templates/team_manage.html")
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	content := string(data)
	required := []string{
		"Удалённые команды",
		"/restore",
		"/hard-delete",
	}
	for _, token := range required {
		if !strings.Contains(content, token) {
			t.Fatalf("expected template to include %q", token)
		}
	}
}
