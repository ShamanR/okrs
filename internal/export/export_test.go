package export

import (
	"strings"
	"testing"
	"time"

	"okrs/internal/domain"
)

func TestMarkdownShortTeam(t *testing.T) {
	period := domain.Period{Name: "Q1 2026"}
	blocks := []TeamBlock{{
		Heading: "Платформа",
		TeamID:  1,
		Goals: []domain.Goal{{
			ID: 10, TeamID: 1, Title: "Снизить P95 latency до 200ms",
			Description: "Оптимизировать критические пути запросов",
			KeyResults: []domain.KeyResult{
				{Title: "P95 latency API gateway", Progress: 0},
				{Title: "Миграция на HTTP/2", Progress: 100},
			},
		}},
	}}
	got := Markdown(period, blocks, Options{Format: FormatShort})
	want := strings.Join([]string{
		"<!-- OKR export · Q1 2026 -->",
		"",
		"# Платформа",
		"",
		"## Снизить P95 latency до 200ms",
		"",
		"Оптимизировать критические пути запросов",
		"",
		"- [ ] P95 latency API gateway",
		"- [x] Миграция на HTTP/2",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("markdown mismatch:\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}

func TestMarkdownShortGoalWithoutDescriptionOrKRs(t *testing.T) {
	got := Markdown(domain.Period{Name: "Q1 2026"}, []TeamBlock{{
		Heading: "Платформа", TeamID: 1,
		Goals: []domain.Goal{{ID: 10, TeamID: 1, Title: "Цель без деталей"}},
	}}, Options{Format: FormatShort})
	want := "<!-- OKR export · Q1 2026 -->\n\n# Платформа\n\n## Цель без деталей\n"
	if got != want {
		t.Fatalf("mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestMarkdownFullGoalAndKR(t *testing.T) {
	blocks := []TeamBlock{{
		Heading: "Платформа", TeamID: 1,
		Goals: []domain.Goal{{
			ID: 10, TeamID: 1, Title: "Снизить P95 latency",
			Description: "Оптимизировать пути",
			Priority:    domain.Priority("P1"), Weight: 20,
			WorkType: domain.WorkType("Delivery"), FocusType: domain.FocusType("TECH_INDEPENDENCE"),
			OwnerText: "Иван, Мария", Progress: 45,
			KeyResults: []domain.KeyResult{{
				Title: "P95 latency", Kind: domain.KRKindNumerical, Weight: 30, Progress: 45,
				ZeroingCriteria: "деградация SLA",
				Description:     "по данным APM",
				Numerical:       &domain.KRNumerical{StartValue: 300, CurrentValue: 250, TargetValue: 200, Unit: "мс"},
				Note:            &domain.KeyResultNote{AuthorName: "Пётр", Text: "работаем"},
			}},
		}},
	}}
	got := Markdown(domain.Period{Name: "Q1 2026"}, blocks, Options{Format: FormatFull})
	for _, want := range []string{
		"## Снизить P95 latency\n",
		"\nP1 · вес 20% · Delivery · Tech Independence · прогресс 45% · драйверы: Иван, Мария\n",
		"\nОптимизировать пути\n",
		"- [ ] P95 latency\n",
		"  - тип: NUMERICAL · 300 → 250 / 200 мс · вес 30% · прогресс 45%\n",
		"  - критерий обнуления: деградация SLA\n",
		"  - описание: по данным APM\n",
		"  - заметка (Пётр):\n",
		"    > работаем\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestMarkdownFullOmitsEmptyFocusAndDrivers(t *testing.T) {
	got := Markdown(domain.Period{Name: "Q1 2026"}, []TeamBlock{{
		Heading: "Платформа", TeamID: 1,
		Goals: []domain.Goal{{
			ID: 1, TeamID: 1, Title: "T", Priority: domain.Priority("P2"), Weight: 10,
			WorkType: domain.WorkType("Discovery"), Progress: 0,
		}},
	}}, Options{Format: FormatFull})
	if !strings.Contains(got, "\nP2 · вес 10% · Discovery · прогресс 0%\n") {
		t.Fatalf("meta line wrong:\n%s", got)
	}
}

func TestMarkdownFullProjectStages(t *testing.T) {
	blocks := []TeamBlock{{
		Heading: "Платформа", TeamID: 1,
		Goals: []domain.Goal{{
			ID: 1, TeamID: 1, Title: "T", Priority: domain.Priority("P1"), Weight: 100,
			WorkType: domain.WorkType("Delivery"), Progress: 50,
			KeyResults: []domain.KeyResult{{
				Title: "Проектный KR", Kind: domain.KRKindProject, Weight: 100, Progress: 50,
				Description: "проектное описание",
				Project: &domain.KRProject{Stages: []domain.KRProjectStage{
					{Title: "Этап A", Weight: 60, IsDone: true},
					{Title: "Этап B", Weight: 40, IsDone: false},
				}},
			}},
		}},
	}}
	got := Markdown(domain.Period{Name: "Q1 2026"}, blocks, Options{Format: FormatFull})
	for _, want := range []string{
		"  - тип: PROJECT · вес 100% · прогресс 50%\n",
		"  - этапы:\n",
		"    - [x] Этап A (вес 60%)\n",
		"    - [ ] Этап B (вес 40%)\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	// description must sit immediately after the type line
	if !strings.Contains(got, "прогресс 50%\n  - описание: проектное описание\n") {
		t.Fatalf("description must follow the type line directly:\n%s", got)
	}
	// stages appear only in full format
	short := Markdown(domain.Period{Name: "Q1 2026"}, blocks, Options{Format: FormatShort})
	if strings.Contains(short, "этапы") {
		t.Fatalf("stages leaked into short format:\n%s", short)
	}
}

func TestMarkdownComments(t *testing.T) {
	resolved := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	created := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	g := domain.Goal{
		ID: 10, TeamID: 1, Title: "Цель",
		Comments: []domain.GoalComment{{
			ID: 1, Text: "Начали профилирование", AuthorName: "Алексей", CreatedAt: created, ResolvedAt: &resolved,
			Replies: []domain.GoalComment{{ID: 2, Text: "ок", AuthorName: "Дмитрий", CreatedAt: created}},
		}},
	}
	blocks := []TeamBlock{{Heading: "Платформа", TeamID: 1, Goals: []domain.Goal{g}}}

	withC := Markdown(domain.Period{Name: "Q1 2026"}, blocks, Options{Format: FormatShort, Comments: true})
	for _, want := range []string{
		"**Комментарии**\n",
		"- Алексей (20.04.2026): Начали профилирование (решено)\n",
		"  - Дмитрий (20.04.2026): ок\n",
	} {
		if !strings.Contains(withC, want) {
			t.Fatalf("missing %q in:\n%s", want, withC)
		}
	}
	noC := Markdown(domain.Period{Name: "Q1 2026"}, blocks, Options{Format: FormatShort, Comments: false})
	if strings.Contains(noC, "Комментарии") {
		t.Fatalf("comments block leaked when Comments=false:\n%s", noC)
	}
}

func TestMarkdownTreeSharedGoalReference(t *testing.T) {
	blocks := []TeamBlock{
		{Heading: "Реклама / Платформа", TeamID: 1, Goals: []domain.Goal{
			{ID: 10, TeamID: 1, Title: "Своя цель"},
		}},
		{Heading: "Реклама / Платформа / Web", TeamID: 2,
			RefGoals: map[int64]string{10: "Платформа"},
			Goals: []domain.Goal{
				{ID: 10, TeamID: 1, Title: "Своя цель"}, // shared into team 2 (owner is team 1)
			}},
	}
	got := Markdown(domain.Period{Name: "Q1 2026"}, blocks, Options{Format: FormatShort})
	if !strings.Contains(got, "# Реклама / Платформа\n") || !strings.Contains(got, "# Реклама / Платформа / Web\n") {
		t.Fatalf("team headings missing:\n%s", got)
	}
	if !strings.Contains(got, "## Своя цель _(общая, владелец: Платформа)_\n") {
		t.Fatalf("shared reference missing:\n%s", got)
	}
	// owner block renders the goal fully (no shared suffix)
	if strings.Count(got, "## Своя цель\n") != 1 {
		t.Fatalf("owner goal should render once as full heading:\n%s", got)
	}
}

// A shared-in goal (owner team differs from the board team) must render fully in goal/team
// scope, where RefGoals is empty — only tree scope collapses it to a reference.
func TestMarkdownSharedGoalRendersFullyWithoutRef(t *testing.T) {
	blocks := []TeamBlock{{
		Heading: "Web", TeamID: 2, // board team 2; goal owned by team 1 (shared in)
		Goals: []domain.Goal{{
			ID: 10, TeamID: 1, Title: "Общая цель", Description: "Описание",
			KeyResults: []domain.KeyResult{{Title: "KR один", Progress: 0}},
			Comments:   []domain.GoalComment{{ID: 1, Text: "замечание", AuthorName: "Аня"}},
		}},
	}}
	got := Markdown(domain.Period{Name: "Q1 2026"}, blocks, Options{Format: FormatShort, Comments: true})
	if strings.Contains(got, "_(общая") {
		t.Fatalf("shared goal must not collapse to a reference without RefGoals:\n%s", got)
	}
	for _, want := range []string{"## Общая цель\n", "Описание", "- [ ] KR один\n", "**Комментарии**\n", "замечание"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in full render:\n%s", want, got)
		}
	}
}

func TestFilename(t *testing.T) {
	period := domain.Period{StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	team := domain.Team{ID: 1, Type: domain.TeamType("unit")}
	cases := map[string]string{
		Filename(period, ScopeGoal, team, 55): "okr-y26q1-g55.md",
		Filename(period, ScopeTeam, team, 0):  "okr-y26q1-u1.md",
		Filename(period, ScopeTree, team, 0):  "okr-y26q1-u1-tree.md",
	}
	for got, want := range cases {
		if got != want {
			t.Fatalf("filename got %q want %q", got, want)
		}
	}
}
