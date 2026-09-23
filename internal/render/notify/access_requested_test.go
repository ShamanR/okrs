package notify_test

import (
	"net/url"
	"testing"

	"okrs/internal/core/event"
	"okrs/internal/render/notify"
)

// Уведомление о заявке называет и заявителя, и пространство: контекста команды
// или цели у него нет, поэтому пространство обязано быть в тексте.
func TestRenderAccessRequested(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindAccessRequested, ActorName: "Пётр", EntityTitle: "Маркетинг", Count: 1,
	})
	if got.Title != "Заявка на доступ" {
		t.Errorf("заголовок: %q", got.Title)
	}
	if got.Body != "Пётр просит доступ к пространству «Маркетинг»" {
		t.Errorf("тело: %q", got.Body)
	}
	if got.Subject != "" {
		t.Errorf("тело уже называет пространство, отдельная строка не нужна: %q", got.Subject)
	}
}

// Ссылка ведёт в очередь заявок админки, а не на цель — у заявки её нет.
func TestTargetURLAccessRequested(t *testing.T) {
	if got := notify.TargetURL(notify.LinkInput{Kind: event.KindAccessRequested}); got != "/admin?section=users&filter=requests" {
		t.Errorf("ссылка на заявку: %q", got)
	}
}

// Ссылка, уходящая наружу, несёт пространство уведомления и ведёт через переход:
// в мессенджере её читает человек, у которого активно может быть другое
// пространство, и обычный путь показал бы ему чужие данные.
func TestTargetURLWithTenantGoesThroughHandoff(t *testing.T) {
	tenant, goal, team := int64(7), int64(5), int64(3)

	got := notify.TargetURL(notify.LinkInput{Kind: event.KindAccessRequested, TenantID: &tenant})
	want := "/open?tenant=7&to=" + url.QueryEscape("/admin?section=users&filter=requests")
	if got != want {
		t.Errorf("заявка: %q, want %q", got, want)
	}

	got = notify.TargetURL(notify.LinkInput{Kind: event.KindGoalFieldsChanged, TenantID: &tenant, GoalID: &goal, TeamID: &team})
	want = "/open?tenant=7&to=" + url.QueryEscape("/?team=3&goal=5")
	if got != want {
		t.Errorf("цель: %q, want %q", got, want)
	}

	// Открывать нечего — переход не появляется из ниоткуда.
	if got := notify.TargetURL(notify.LinkInput{Kind: event.KindGoalDeleted, TenantID: &tenant, GoalID: &goal, GoalMissing: true}); got != "" {
		t.Errorf("удалённая цель: %q, want пусто", got)
	}
}

// Остальные kind ведут туда же, куда и раньше: поле Kind не меняет их ссылок.
func TestTargetURLOtherKindsUnchanged(t *testing.T) {
	goal, team := int64(5), int64(3)
	for _, in := range []notify.LinkInput{
		{Kind: event.KindGoalFieldsChanged, GoalID: &goal, TeamID: &team},
		{GoalID: &goal, TeamID: &team},
	} {
		if got := notify.TargetURL(in); got != "/?team=3&goal=5" {
			t.Errorf("%s: %q", in.Kind, got)
		}
	}
	if got := notify.TargetURL(notify.LinkInput{Kind: event.KindCommentAdded}); got != "" {
		t.Errorf("без цели ссылки нет: %q", got)
	}
}
