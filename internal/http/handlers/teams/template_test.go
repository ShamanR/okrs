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
		"data-okr-summary",
		"data-okr-goals",
	}
	for _, token := range required {
		if !strings.Contains(content, token) {
			t.Fatalf("expected template to include %q", token)
		}
	}
}
