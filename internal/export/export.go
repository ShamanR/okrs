package export

import (
	"strconv"
	"strings"

	"okrs/internal/domain"
)

type Format string

const (
	FormatShort Format = "short"
	FormatFull  Format = "full"
)

type Options struct {
	Format   Format
	Comments bool
}

type Scope string

const (
	ScopeGoal Scope = "goal"
	ScopeTeam Scope = "team"
	ScopeTree Scope = "tree"
)

type TeamBlock struct {
	Heading string
	TeamID  int64
	Goals   []domain.Goal
	// RefGoals marks goals that should render as a shared reference (title + owner) instead of
	// their full body. Populated only in tree scope for goals whose owner is another team, so the
	// full goal appears once (under its owner). Empty in goal/team scope — everything renders full.
	RefGoals map[int64]string // goalID -> owner team name
}

// Markdown renders the export document. Blocks are rendered in the given order.
func Markdown(period domain.Period, blocks []TeamBlock, opts Options) string {
	var b strings.Builder
	b.WriteString("<!-- OKR export · " + oneline(period.Name) + " -->\n")
	for _, block := range blocks {
		b.WriteString("\n# " + oneline(block.Heading) + "\n")
		for _, g := range block.Goals {
			writeGoal(&b, block, g, opts)
		}
	}
	return b.String()
}

// oneline collapses newlines, carriage returns and tabs to single spaces so user-provided
// identifiers (titles, team names) placed on structural Markdown lines cannot inject extra
// headings, list items or blocks. Descriptions/notes are intentionally left as raw Markdown.
func oneline(s string) string {
	return strings.TrimSpace(strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ", "\t", " ").Replace(s))
}

func writeGoal(b *strings.Builder, block TeamBlock, g domain.Goal, opts Options) {
	if owner, ok := block.RefGoals[g.ID]; ok {
		b.WriteString("\n## " + oneline(g.Title) + " _(общая, владелец: " + oneline(owner) + ")_\n")
		return
	}
	b.WriteString("\n## " + oneline(g.Title) + "\n")
	if opts.Format == FormatFull {
		b.WriteString("\n" + goalMetaLine(g) + "\n")
	}
	if g.Description != "" {
		b.WriteString("\n" + g.Description + "\n")
	}
	if len(g.KeyResults) > 0 {
		b.WriteString("\n")
		for _, kr := range g.KeyResults {
			writeKR(b, kr, opts)
		}
	}
	if opts.Comments {
		writeComments(b, g)
	}
}

func writeComments(b *strings.Builder, g domain.Goal) {
	tasks := make([]domain.GoalComment, 0, len(g.Comments))
	for _, c := range g.Comments {
		if c.ParentID == nil {
			tasks = append(tasks, c)
		}
	}
	if len(tasks) == 0 {
		return
	}
	b.WriteString("\n**Комментарии**\n")
	for _, task := range tasks {
		suffix := ""
		if task.ResolvedAt != nil {
			suffix = " (решено)"
		}
		b.WriteString("- " + task.AuthorName + " (" + task.CreatedAt.UTC().Format("02.01.2006") + "): " + task.Text + suffix + "\n")
		for _, r := range task.Replies {
			b.WriteString("  - " + r.AuthorName + " (" + r.CreatedAt.UTC().Format("02.01.2006") + "): " + r.Text + "\n")
		}
	}
}

func goalMetaLine(g domain.Goal) string {
	segs := []string{
		string(g.Priority),
		"вес " + strconv.Itoa(g.Weight) + "%",
		string(g.WorkType),
	}
	if f := focusLabel(g.FocusType); f != "" {
		segs = append(segs, f)
	}
	segs = append(segs, "прогресс "+strconv.Itoa(g.Progress)+"%")
	line := strings.Join(segs, " · ")
	if g.OwnerText != "" {
		line += " · драйверы: " + g.OwnerText
	}
	return line
}

func writeKR(b *strings.Builder, kr domain.KeyResult, opts Options) {
	box := " "
	if kr.Progress == 100 {
		box = "x"
	}
	b.WriteString("- [" + box + "] " + oneline(kr.Title) + "\n")
	if opts.Format != FormatFull {
		return
	}
	detail := "тип: " + string(kr.Kind) + numericalSuffix(kr) +
		" · вес " + strconv.Itoa(kr.Weight) + "% · прогресс " + strconv.Itoa(kr.Progress) + "%"
	b.WriteString("  - " + detail + "\n")
	if kr.Description != "" {
		b.WriteString("  - описание: " + kr.Description + "\n")
	}
	if kr.Kind == domain.KRKindProject && kr.Project != nil && len(kr.Project.Stages) > 0 {
		b.WriteString("  - этапы:\n")
		for _, st := range kr.Project.Stages {
			sbox := " "
			if st.IsDone {
				sbox = "x"
			}
			b.WriteString("    - [" + sbox + "] " + oneline(st.Title) + " (вес " + strconv.Itoa(st.Weight) + "%)\n")
		}
	}
	if kr.ZeroingCriteria != "" {
		b.WriteString("  - критерий обнуления: " + kr.ZeroingCriteria + "\n")
	}
	if kr.Note != nil {
		b.WriteString("  - заметка (" + kr.Note.AuthorName + "):\n")
		for _, line := range strings.Split(kr.Note.Text, "\n") {
			b.WriteString("    > " + line + "\n")
		}
	}
}

func numericalSuffix(kr domain.KeyResult) string {
	if kr.Kind != domain.KRKindNumerical || kr.Numerical == nil {
		return ""
	}
	n := kr.Numerical
	s := " · " + formatNumber(n.StartValue) + " → " + formatNumber(n.CurrentValue) + " / " + formatNumber(n.TargetValue)
	if n.Unit != "" {
		s += " " + n.Unit
	}
	return s
}

func focusLabel(f domain.FocusType) string {
	if f == "" {
		return ""
	}
	words := strings.Split(strings.ToLower(string(f)), "_")
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

func formatNumber(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// Filename builds the suggested download filename for the export.
func Filename(period domain.Period, scope Scope, team domain.Team, goalID int64) string {
	start := period.StartDate.UTC()
	quarter := (int(start.Month())-1)/3 + 1
	base := "y" + pad2(start.Year()%100) + "q" + strconv.Itoa(quarter)
	var code string
	switch scope {
	case ScopeGoal:
		code = "g" + strconv.FormatInt(goalID, 10)
	case ScopeTree:
		code = typeLetter(team) + strconv.FormatInt(team.ID, 10) + "-tree"
	default: // ScopeTeam
		code = typeLetter(team) + strconv.FormatInt(team.ID, 10)
	}
	return "okr-" + base + "-" + code + ".md"
}

func pad2(n int) string {
	s := strconv.Itoa(n)
	if len(s) < 2 {
		return "0" + s
	}
	return s
}

func typeLetter(team domain.Team) string {
	t := string(team.Type)
	if t == "" {
		return "t"
	}
	return t[:1]
}
