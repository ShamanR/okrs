package domain

import (
	"sort"
	"time"
)

// PeriodView — период с вычисленными на чтении полями дерева и статусом.
type PeriodView struct {
	Period
	ParentID *int64
	Depth    int
	Status   PeriodStatus
}

// BuildPeriodViews вычисляет родителя (по строгому вхождению интервалов),
// глубину, статус и порядок отображения. Порядок: актуальные+будущие корни
// сверху, затем закрытые, затем архивные; новые годы выше; дети под родителем
// (в актуальных/будущих — по возрастанию дат, в прошедших — по убыванию).
func BuildPeriodViews(periods []Period, now time.Time) []PeriodView {
	n := len(periods)
	status := make(map[int64]PeriodStatus, n)
	for _, p := range periods {
		status[p.ID] = PeriodStatusFor(p, now)
	}

	// span в днях; для тай-брейка узости.
	span := func(p Period) int {
		return int(p.EndDate.Sub(p.StartDate).Hours() / 24)
	}
	contains := func(a, c Period) bool {
		if a.ID == c.ID {
			return false
		}
		if a.StartDate.After(c.StartDate) || a.EndDate.Before(c.EndDate) {
			return false
		}
		return a.StartDate.Before(c.StartDate) || a.EndDate.After(c.EndDate)
	}

	parent := make(map[int64]*int64, n)
	for _, c := range periods {
		var best *Period
		for i := range periods {
			a := periods[i]
			if !contains(a, c) {
				continue
			}
			if best == nil {
				b := a
				best = &b
				continue
			}
			// узость: меньший span, затем позднейший start, ранний end, меньший id.
			as, bs := span(a), span(*best)
			switch {
			case as != bs:
				if as < bs {
					b := a
					best = &b
				}
			case !a.StartDate.Equal(best.StartDate):
				if a.StartDate.After(best.StartDate) {
					b := a
					best = &b
				}
			case !a.EndDate.Equal(best.EndDate):
				if a.EndDate.Before(best.EndDate) {
					b := a
					best = &b
				}
			default:
				if a.ID < best.ID {
					b := a
					best = &b
				}
			}
		}
		if best != nil {
			pid := best.ID
			parent[c.ID] = &pid
		} else {
			parent[c.ID] = nil
		}
	}

	// depth по цепочке родителей.
	depth := make(map[int64]int, n)
	var calcDepth func(id int64) int
	calcDepth = func(id int64) int {
		if dp, ok := depth[id]; ok {
			return dp
		}
		pid := parent[id]
		if pid == nil {
			depth[id] = 0
			return 0
		}
		dp := calcDepth(*pid) + 1
		depth[id] = dp
		return dp
	}
	for _, p := range periods {
		calcDepth(p.ID)
	}

	byID := make(map[int64]Period, n)
	for _, p := range periods {
		byID[p.ID] = p
	}
	children := make(map[int64][]int64)
	var roots []int64
	for _, p := range periods {
		if pid := parent[p.ID]; pid != nil {
			children[*pid] = append(children[*pid], p.ID)
		} else {
			roots = append(roots, p.ID)
		}
	}

	rootRank := func(s PeriodStatus) int {
		switch s {
		case PeriodStatusClosed:
			return 1
		case PeriodStatusArchived:
			return 2
		default: // future, active
			return 0
		}
	}
	sort.SliceStable(roots, func(i, j int) bool {
		a, b := byID[roots[i]], byID[roots[j]]
		ra, rb := rootRank(status[a.ID]), rootRank(status[b.ID])
		if ra != rb {
			return ra < rb
		}
		if !a.StartDate.Equal(b.StartDate) {
			return a.StartDate.After(b.StartDate) // новые выше
		}
		return a.ID < b.ID
	})

	sortChildren := func(parentID int64) {
		kids := children[parentID]
		asc := status[parentID] == PeriodStatusFuture || status[parentID] == PeriodStatusActive
		sort.SliceStable(kids, func(i, j int) bool {
			a, b := byID[kids[i]], byID[kids[j]]
			if !a.StartDate.Equal(b.StartDate) {
				if asc {
					return a.StartDate.Before(b.StartDate)
				}
				return a.StartDate.After(b.StartDate)
			}
			return a.ID < b.ID
		})
	}

	out := make([]PeriodView, 0, n)
	var walk func(id int64)
	walk = func(id int64) {
		p := byID[id]
		var pid *int64
		if v := parent[id]; v != nil {
			cp := *v
			pid = &cp
		}
		out = append(out, PeriodView{Period: p, ParentID: pid, Depth: depth[id], Status: status[id]})
		sortChildren(id)
		for _, kid := range children[id] {
			walk(kid)
		}
	}
	for _, r := range roots {
		walk(r)
	}
	return out
}
