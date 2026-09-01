package notifications

import (
	"testing"

	storenotif "okrs/internal/store/notifications"
)

// targetURL должен собирать ссылку в формате, который трекер реально умеет читать.
// Канонический формат deep-link задаёт buildTargetURL в web/static/ui.js (`/?team=&
// period=&goal=&kr=&comment=`), а разбирает readURLNav в web/static/tracker.js через
// p.get('team')/'period'/'goal'/'kr'/'comment'. Имена и порядок параметров ниже
// обязаны совпадать с buildTargetURL — если кто-то "исправит" эти проверки под
// другую реализацию targetURL, он тем самым воспроизведёт баг, который этот тест
// ловит: клик по колокольчику вёл в никуда, потому что сервер слал goal_id/team_id/
// period_id, а трекер эти имена не читает вовсе.
func idPtr(v int64) *int64 { return &v }

func TestTargetURL(t *testing.T) {
	cases := []struct {
		name string
		n    storenotif.Notification
		want string
	}{
		{
			name: "только цель",
			n:    storenotif.Notification{GoalID: idPtr(5), GoalTitle: "Снизить отток"},
			want: "/?goal=5",
		},
		{
			name: "цель с командой и периодом",
			n:    storenotif.Notification{GoalID: idPtr(5), TeamID: idPtr(9), PeriodID: idPtr(3), GoalTitle: "Снизить отток"},
			want: "/?team=9&period=3&goal=5",
		},
		{
			name: "цель с ключевым результатом",
			n:    storenotif.Notification{GoalID: idPtr(5), KRID: idPtr(7), GoalTitle: "Снизить отток"},
			want: "/?goal=5&kr=7",
		},
		{
			name: "цель с комментарием",
			n:    storenotif.Notification{GoalID: idPtr(5), CommentID: idPtr(11), GoalTitle: "Снизить отток"},
			want: "/?goal=5&comment=11",
		},
		{
			name: "полный набор",
			n: storenotif.Notification{
				GoalID: idPtr(5), TeamID: idPtr(9), PeriodID: idPtr(3),
				KRID: idPtr(7), CommentID: idPtr(11), GoalTitle: "Снизить отток",
			},
			want: "/?team=9&period=3&goal=5&kr=7&comment=11",
		},
		{
			name: "нет цели — ссылки нет",
			n:    storenotif.Notification{TeamID: idPtr(9), PeriodID: idPtr(3)},
			want: "",
		},
		{
			// Идентификатор цели переживает саму цель: уведомление сохраняет якорь
			// после жёсткого удаления, а goal_deleted и вовсе всегда про исчезнувшую
			// цель. Пустое название из LEFT JOIN — признак того, что вести некуда.
			name: "цель удалена — ссылки нет",
			n:    storenotif.Notification{GoalID: idPtr(5), TeamID: idPtr(9), GoalTitle: ""},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := targetURL(tc.n)
			if got != tc.want {
				t.Fatalf("targetURL() = %q, want %q", got, tc.want)
			}
		})
	}
}
