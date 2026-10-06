package notifications_test

import (
	"testing"

	"okrs/internal/store/notifications"
)

// Схлопывание обновляет payload_json последним событием, поэтому вместе с ним
// обязано обновляться и team_id: это не формулировка, а маршрут — доска, которую
// уведомление откроет.
//
// Расхождение достижимо на общей цели: один и тот же руководитель может быть
// найден через разные команды-участники двумя событиями одного окна. Если
// оставить первую команду рядом с последним payload, то после снятия команды со
// цели следующая правка перезапишет payload на тот, который про команды ничего
// не говорит, а team_id продолжит указывать на доску, с которой цель уже ушла.
func TestCoalesceRefreshesTeamID(t *testing.T) {
	repo, ctx, scope, cleanup := newRepo(t)
	defer cleanup()

	const key = "goal_changed:goal:10:2:100"
	first, second := int64(7), int64(9)

	in := input(key)
	in.TeamID = &first
	if _, err := repo.Insert(ctx, scope, in); err != nil {
		t.Fatalf("первая вставка: %v", err)
	}

	in = input(key)
	in.TeamID = &second
	in.Payload = map[string]any{"removed_team_ids": []any{first}}
	created, err := repo.Insert(ctx, scope, in)
	if err != nil {
		t.Fatalf("вторая вставка: %v", err)
	}
	if created {
		t.Fatal("второе событие того же окна обязано схлопнуться, а не создать строку")
	}

	items, _, err := repo.List(ctx, scope, 1, notifications.ListFilter{Limit: 20})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ожидалась одна строка, got %d", len(items))
	}
	got := items[0]
	if got.CoalesceCount != 2 {
		t.Errorf("счётчик повторов = %d, want 2", got.CoalesceCount)
	}
	if got.TeamID == nil || *got.TeamID != second {
		t.Fatalf("team_id обязан следовать за payload последнего события: got %v, want %d",
			teamOrNil(got.TeamID), second)
	}
}

func teamOrNil(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}
