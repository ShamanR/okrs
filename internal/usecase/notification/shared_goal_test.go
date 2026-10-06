package notification_test

import (
	"context"
	"errors"
	"sort"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	"okrs/internal/store/notificationprefs"
	"okrs/internal/store/notifications"
	notificationuc "okrs/internal/usecase/notification"
)

// fakeGoalTeams is the share composition the fan-out reads. It counts calls, because
// reading it per event instead of per group is the N+1 this design exists to avoid.
type fakeGoalTeams struct {
	teams map[int64][]int64
	calls int
	// goalIDArgs records every batch asked for, so a test can assert the ids were
	// deduplicated before the query.
	goalIDArgs [][]int64
	err        error
}

func (f *fakeGoalTeams) TeamIDsByGoalIDs(_ context.Context, _ domain.TenantScope, goalIDs []int64) (map[int64][]int64, error) {
	f.calls++
	f.goalIDArgs = append(f.goalIDArgs, append([]int64(nil), goalIDs...))
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[int64][]int64, len(goalIDs))
	for _, id := range goalIDs {
		if t := f.teams[id]; len(t) > 0 {
			out[id] = t
		}
	}
	return out, nil
}

// teamAwarePrefs answers one recipient per TEAM: user id 1000+teamID, found at the
// distance the test asked for. That ties a resolved recipient to the team it came
// through, which is what the dedup rule and the row's team are about.
type teamAwarePrefs struct {
	distanceByTeam map[int64]int
	targets        []notificationprefs.Target
	calls          int
}

func (f *teamAwarePrefs) Resolve(_ context.Context, _ domain.TenantScope, _ string, targets []notificationprefs.Target) ([]notificationprefs.Recipient, error) {
	f.calls++
	f.targets = append(f.targets, targets...)
	out := make([]notificationprefs.Recipient, 0, len(targets))
	for i, t := range targets {
		out = append(out, notificationprefs.Recipient{
			Ord: i, UserID: 1000 + t.TeamID, TeamID: t.TeamID, Distance: f.distanceByTeam[t.TeamID],
		})
	}
	return out, nil
}

func (f *teamAwarePrefs) ResolveAddressed(_ context.Context, _ domain.TenantScope, _ string, userIDs []int64) ([]notificationprefs.Recipient, error) {
	out := make([]notificationprefs.Recipient, 0, len(userIDs))
	for i, id := range userIDs {
		out = append(out, notificationprefs.Recipient{Ord: i, UserID: id})
	}
	return out, nil
}

func (*teamAwarePrefs) ResolveTenantAdmins(context.Context, domain.TenantScope, string, []int64) ([]notificationprefs.Recipient, error) {
	return nil, nil
}

func (*teamAwarePrefs) DeliveryDefaults(context.Context, domain.TenantScope) (map[string]bool, error) {
	return map[string]bool{notificationprefs.ChannelInApp: true}, nil
}

// sharedUC wires the fan-out over a goal whose teams are given by `teams`.
func sharedUC(teams map[int64][]int64) (*notificationuc.UseCase, *fakeWriter, *teamAwarePrefs, *fakeGoalTeams) {
	w := &fakeWriter{}
	p := &teamAwarePrefs{distanceByTeam: map[int64]int{}}
	g := &fakeGoalTeams{teams: teams}
	return notificationuc.New(notificationuc.Deps{Notifications: w, Prefs: p, GoalTeams: g}), w, p, g
}

func resolvedTeams(targets []notificationprefs.Target) []int64 {
	out := make([]int64, 0, len(targets))
	for _, t := range targets {
		out = append(out, t.TeamID)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func rowUsers(rows []notifications.InsertInput) []int64 {
	out := make([]int64, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.UserID)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func equalInt64s(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Правка общей цели адресуется от каждой команды-участника, а не только от
// команды-владельца, которую несёт событие.
func TestGoalChangeIsAddressedFromEveryParticipantTeam(t *testing.T) {
	uc, w, p, _ := sharedUC(map[int64][]int64{10: {3, 5, 8}})
	if err := uc.Handle(context.Background(), []event.Event{
		event.GoalFieldsChanged{Meta: meta(), GoalID: 10, Title: "Цель"},
	}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if got, want := resolvedTeams(p.targets), []int64{3, 5, 8}; !equalInt64s(got, want) {
		t.Fatalf("резолв должен быть запрошен от всех команд-участников: got %v, want %v", got, want)
	}
	if got, want := rowUsers(w.rows), []int64{1003, 1005, 1008}; !equalInt64s(got, want) {
		t.Fatalf("строки: got %v, want %v", got, want)
	}
}

// Для нерасшаренной цели поведение прежнее: одна команда, один target.
func TestUnsharedGoalStillAddressedFromItsOwnTeamOnly(t *testing.T) {
	uc, w, p, _ := sharedUC(map[int64][]int64{10: {3}})
	if err := uc.Handle(context.Background(), []event.Event{
		event.GoalFieldsChanged{Meta: meta(), GoalID: 10, Title: "Цель"},
	}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if got, want := resolvedTeams(p.targets), []int64{3}; !equalInt64s(got, want) {
		t.Fatalf("targets: got %v, want %v", got, want)
	}
	if len(w.rows) != 1 {
		t.Fatalf("ожидалась одна строка, got %d", len(w.rows))
	}
}

// Состав команд читается одним вызовом на группу событий и с уникальными id —
// иначе батч из 50 событий по одной цели даёт 50 запросов (правило 9 CLAUDE.md).
func TestShareCompositionReadOncePerGroupWithUniqueGoalIDs(t *testing.T) {
	uc, _, _, g := sharedUC(map[int64][]int64{10: {3, 5}, 11: {3}})
	evs := make([]event.Event, 0, 6)
	for i := 0; i < 3; i++ {
		evs = append(evs,
			event.GoalFieldsChanged{Meta: meta(), GoalID: 10, Title: "Цель"},
			event.GoalFieldsChanged{Meta: meta(), GoalID: 11, Title: "Цель"},
		)
	}
	if err := uc.Handle(context.Background(), evs); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if g.calls != 1 {
		t.Fatalf("состав команд прочитан %d раз на 6 событий одного типа, want 1", g.calls)
	}
	if got := g.goalIDArgs[0]; len(got) != 2 {
		t.Fatalf("в запрос должны уйти уникальные id целей, got %v", got)
	}
}

// Руководитель, найденный через две команды-участника, получает ОДНУ строку:
// две разошлись бы в один и тот же ключ схлопывания и показали бы ему «×2».
func TestLeadFoundThroughTwoTeamsGetsOneRow(t *testing.T) {
	w := &fakeWriter{}
	// Все три команды (владелец 3 из meta() и два участника) резолвятся в одного
	// и того же человека, найденного с разной дистанции.
	p := &sameUserPrefs{distanceByTeam: map[int64]int{3: 3, 5: 1, 8: 2}}
	g := &fakeGoalTeams{teams: map[int64][]int64{10: {3, 5, 8}}}
	uc := notificationuc.New(notificationuc.Deps{Notifications: w, Prefs: p, GoalTeams: g})
	if err := uc.Handle(context.Background(), []event.Event{
		event.GoalFieldsChanged{Meta: meta(), GoalID: 10, Title: "Цель"},
	}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 1 {
		t.Fatalf("ожидалась одна строка на получателя, got %d: %+v", len(w.rows), w.rows)
	}
	if got := w.rows[0].TeamID; got == nil || *got != 5 {
		t.Fatalf("в записи обязана остаться ближайшая команда (distance 1), got %v", derefTeam(got))
	}
}

func derefTeam(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

// При равной дистанции выбор обязан быть детерминированным — наименьший id.
func TestEqualDistanceTieIsResolvedByLowestTeamID(t *testing.T) {
	for _, order := range [][]int64{{3, 8, 5}, {3, 5, 8}} {
		w := &fakeWriter{}
		// Команда-владелец 3 дальше всех, а 5 и 8 равноудалены — выбор между ними
		// и есть то, что обязано быть детерминированным.
		p := &sameUserPrefs{distanceByTeam: map[int64]int{3: 4, 5: 1, 8: 1}}
		g := &fakeGoalTeams{teams: map[int64][]int64{10: order}}
		uc := notificationuc.New(notificationuc.Deps{Notifications: w, Prefs: p, GoalTeams: g})
		if err := uc.Handle(context.Background(), []event.Event{
			event.GoalFieldsChanged{Meta: meta(), GoalID: 10, Title: "Цель"},
		}); err != nil {
			t.Fatalf("handle: %v", err)
		}
		if len(w.rows) != 1 {
			t.Fatalf("порядок %v: ожидалась одна строка, got %d", order, len(w.rows))
		}
		if got := w.rows[0].TeamID; got == nil || *got != 5 {
			t.Fatalf("порядок %v: результат обязан не зависеть от порядка строк, got %v", order, derefTeam(got))
		}
	}
}

// Запись ведёт на доску той команды, через которую найден получатель: у
// руководителя команды-участника доступа к команде-владельцу может не быть.
func TestRowTeamIsTheTeamTheRecipientWasFoundThrough(t *testing.T) {
	uc, w, _, _ := sharedUC(map[int64][]int64{10: {3, 8}})
	if err := uc.Handle(context.Background(), []event.Event{
		event.GoalFieldsChanged{Meta: meta(), GoalID: 10, Title: "Цель"},
	}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	for _, r := range w.rows {
		if r.TeamID == nil {
			t.Fatalf("у строки нет команды: %+v", r)
		}
		if want := r.UserID - 1000; *r.TeamID != want {
			t.Errorf("получатель %d найден через команду %d, а записана %d", r.UserID, want, *r.TeamID)
		}
	}
}

// Добавление команды уведомляет и её саму, и остальных участников.
func TestGoalSharedNotifiesAddedAndRemainingTeams(t *testing.T) {
	uc, w, p, _ := sharedUC(map[int64][]int64{10: {3, 5, 8}})
	if err := uc.Handle(context.Background(), []event.Event{
		event.GoalShared{Meta: meta(), GoalID: 10, Title: "Цель", SharedWithTeamIDs: []int64{8}},
	}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if got, want := resolvedTeams(p.targets), []int64{3, 5, 8}; !equalInt64s(got, want) {
		t.Fatalf("targets: got %v, want %v", got, want)
	}
	for _, r := range w.rows {
		if r.Type != notificationprefs.TypeGoalChanged {
			t.Errorf("тип = %q, want %q", r.Type, notificationprefs.TypeGoalChanged)
		}
	}
	if len(w.rows) != 3 {
		t.Fatalf("ожидались три строки, got %d", len(w.rows))
	}
}

// Снятая команда уже отсутствует в составе — её идентификатор берётся из самого
// события, иначе её руководитель об уходе цели не узнает. Проверяются все три
// формы payload GoalUnshared.
func TestGoalUnsharedNotifiesTheRemovedTeamInEveryPayloadShape(t *testing.T) {
	cases := map[string]event.GoalUnshared{
		"полная замена состава": {Meta: meta(), GoalID: 10, Title: "Цель", UnsharedTeamIDs: []int64{8}},
		"одиночное удаление":    {Meta: meta(), GoalID: 10, Title: "Цель", UnsharedTeamID: 8},
		"отказ от участия":      {Meta: meta(), GoalID: 10, Title: "Цель", DeclinedByTeamID: 8},
	}
	for name, ev := range cases {
		// Состав уже без команды 8 — именно так его видит обработчик.
		uc, w, p, _ := sharedUC(map[int64][]int64{10: {3, 5}})
		if err := uc.Handle(context.Background(), []event.Event{ev}); err != nil {
			t.Fatalf("%s: handle: %v", name, err)
		}
		if got, want := resolvedTeams(p.targets), []int64{3, 5, 8}; !equalInt64s(got, want) {
			t.Fatalf("%s: снятая команда обязана попасть в адресацию: got %v, want %v", name, got, want)
		}
		if got, want := rowUsers(w.rows), []int64{1003, 1005, 1008}; !equalInt64s(got, want) {
			t.Fatalf("%s: строки: got %v, want %v", name, got, want)
		}
	}
}

// Решение замечания к ОБЩЕЙ цели порождает два уведомления: адресное автору и
// командное участникам.
func TestCommentResolvedOnSharedGoalAlsoNotifiesParticipants(t *testing.T) {
	uc, w, _, _ := sharedUC(map[int64][]int64{10: {3, 5}})
	if err := uc.Handle(context.Background(), []event.Event{
		event.CommentResolved{Meta: meta(), GoalID: 10, CommentID: 4, GoalTitle: "Цель", AuthorUserID: 99},
	}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	byType := map[string][]int64{}
	for _, r := range w.rows {
		byType[r.Type] = append(byType[r.Type], r.UserID)
	}
	if got := byType[notificationprefs.TypeMyCommentResolved]; !equalInt64s(got, []int64{99}) {
		t.Errorf("адресное уведомление автору: got %v", got)
	}
	got := byType[notificationprefs.TypeGoalComment]
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	if !equalInt64s(got, []int64{1003, 1005}) {
		t.Errorf("командное уведомление участникам: got %v", got)
	}
}

// У необщей цели решение замечания остаётся личным делом автора.
func TestCommentResolvedOnUnsharedGoalNotifiesOnlyTheAuthor(t *testing.T) {
	uc, w, _, _ := sharedUC(map[int64][]int64{10: {3}})
	if err := uc.Handle(context.Background(), []event.Event{
		event.CommentResolved{Meta: meta(), GoalID: 10, CommentID: 4, GoalTitle: "Цель", AuthorUserID: 99},
	}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) != 1 || w.rows[0].Type != notificationprefs.TypeMyCommentResolved {
		t.Fatalf("ожидалось только адресное уведомление, got %+v", w.rows)
	}
}

// Чек-ин и замечание к общей цели доходят до всех команд-участников.
func TestCheckInAndCommentReachEveryParticipantTeam(t *testing.T) {
	for name, ev := range map[string]event.Event{
		"чек-ин":      event.KRCheckedIn{Meta: meta(), GoalID: 10, KRID: 20, KRTitle: "KR"},
		"замечание":   event.CommentAdded{Meta: meta(), GoalID: 10, CommentID: 4, GoalTitle: "Цель", Text: "hi"},
		"ответ в ней": event.ReplyAdded{Meta: meta(), GoalID: 10, CommentID: 5, ParentCommentID: 4, GoalTitle: "Цель", Text: "re"},
	} {
		uc, w, _, _ := sharedUC(map[int64][]int64{10: {3, 5}})
		if err := uc.Handle(context.Background(), []event.Event{ev}); err != nil {
			t.Fatalf("%s: handle: %v", name, err)
		}
		if got, want := rowUsers(w.rows), []int64{1003, 1005}; !equalInt64s(got, want) {
			t.Fatalf("%s: got %v, want %v", name, got, want)
		}
	}
}

// Ошибка чтения состава сообщается, но батч не теряется: обработчик асинхронный,
// ретраев нет, и возврат ошибки стоил бы уведомления ВСЕМ — включая команду
// события, которой состав не нужен вовсе.
func TestShareLookupFailureIsReportedButKeepsTheEventTeam(t *testing.T) {
	w := &fakeWriter{}
	p := &teamAwarePrefs{distanceByTeam: map[int64]int{}}
	g := &fakeGoalTeams{err: errors.New("БД недоступна")}
	uc := notificationuc.New(notificationuc.Deps{Notifications: w, Prefs: p, GoalTeams: g})
	err := uc.Handle(context.Background(), []event.Event{
		event.GoalFieldsChanged{Meta: meta(), GoalID: 10, Title: "Цель"},
	})
	if err == nil {
		t.Fatal("ошибка чтения состава обязана быть видна")
	}
	if got, want := resolvedTeams(p.targets), []int64{3}; !equalInt64s(got, want) {
		t.Fatalf("аудитория обязана схлопнуться к команде события: got %v, want %v", got, want)
	}
	if len(w.rows) != 1 {
		t.Fatalf("команда события обязана получить уведомление, got %d строк", len(w.rows))
	}
}

// Сборка без резолвера состава ведёт себя ровно как до появления общих целей.
func TestBuildWithoutGoalTeamsAddressesTheEventTeamOnly(t *testing.T) {
	w := &fakeWriter{}
	p := &teamAwarePrefs{distanceByTeam: map[int64]int{}}
	uc := notificationuc.New(notificationuc.Deps{Notifications: w, Prefs: p})
	if err := uc.Handle(context.Background(), []event.Event{
		event.GoalFieldsChanged{Meta: meta(), GoalID: 10, Title: "Цель"},
	}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if got, want := resolvedTeams(p.targets), []int64{3}; !equalInt64s(got, want) {
		t.Fatalf("targets: got %v, want %v", got, want)
	}
}

// Payload несёт изменённые команды — без него рендер не отличит «вашу команду
// добавили» от «состав изменился».
func TestCompositionPayloadCarriesTheChangedTeams(t *testing.T) {
	uc, w, _, _ := sharedUC(map[int64][]int64{10: {3}})
	if err := uc.Handle(context.Background(), []event.Event{
		event.GoalShared{Meta: meta(), GoalID: 10, Title: "Цель", SharedWithTeamIDs: []int64{8, 9}},
	}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(w.rows) == 0 {
		t.Fatal("ожидались строки")
	}
	raw, ok := w.rows[0].Payload["added_team_ids"].([]any)
	if !ok || len(raw) != 2 {
		t.Fatalf("payload обязан нести добавленные команды, got %+v", w.rows[0].Payload)
	}
	if _, present := w.rows[0].Payload["removed_team_ids"]; present {
		t.Errorf("добавление не должно писать ключ снятых команд: %+v", w.rows[0].Payload)
	}
}

// Регресс: схлопывание замораживает kind первого события, но берёт payload
// последнего. Добавление и снятие по одной цели в одном окне попадают в одну
// строку, и текст обязан описывать последнее изменение, а не первое. Поэтому
// глагол живёт в payload — отдельными ключами, — а не выводится из kind.
func TestCompositionPayloadKeysAreVerbSpecific(t *testing.T) {
	uc, w, _, _ := sharedUC(map[int64][]int64{10: {3}})
	if err := uc.Handle(context.Background(), []event.Event{
		event.GoalShared{Meta: meta(), GoalID: 10, Title: "Цель", SharedWithTeamIDs: []int64{8}},
		event.GoalUnshared{Meta: meta(), GoalID: 10, Title: "Цель", UnsharedTeamIDs: []int64{9}},
	}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	var sharedPayload, unsharedPayload map[string]any
	for _, r := range w.rows {
		switch r.Kind {
		case "goal_shared":
			sharedPayload = r.Payload
		case "goal_unshared":
			unsharedPayload = r.Payload
		}
	}
	if sharedPayload == nil || unsharedPayload == nil {
		t.Fatalf("ожидались строки обоих событий, got %d", len(w.rows))
	}
	if _, ok := sharedPayload["added_team_ids"]; !ok {
		t.Errorf("goal_shared обязан писать added_team_ids: %+v", sharedPayload)
	}
	if _, ok := unsharedPayload["removed_team_ids"]; !ok {
		t.Errorf("goal_unshared обязан писать removed_team_ids: %+v", unsharedPayload)
	}
	// Ключи не пересекаются — иначе перезапись payload при схлопывании снова
	// перепутала бы глагол.
	if _, ok := unsharedPayload["added_team_ids"]; ok {
		t.Errorf("снятие не должно писать ключ добавленных команд: %+v", unsharedPayload)
	}
}

// Решение замечания к общей цели кладёт в командную строку тип goal_comment, а в
// адресную — my_comment_resolved: именно по типу рендер различает «решил ваш
// комментарий» и «решил замечание к цели».
func TestResolvedCommentRowsCarryTheirOwnType(t *testing.T) {
	uc, w, _, _ := sharedUC(map[int64][]int64{10: {3, 5}})
	if err := uc.Handle(context.Background(), []event.Event{
		event.CommentResolved{Meta: meta(), GoalID: 10, CommentID: 4, GoalTitle: "Цель", AuthorUserID: 99},
	}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	for _, r := range w.rows {
		if r.Kind != "comment_resolved" {
			t.Fatalf("неожиданный kind: %q", r.Kind)
		}
		want := notificationprefs.TypeGoalComment
		if r.UserID == 99 {
			want = notificationprefs.TypeMyCommentResolved
		}
		if r.Type != want {
			t.Errorf("получатель %d: тип = %q, want %q", r.UserID, r.Type, want)
		}
	}
}

// sameUserPrefs резолвит ЛЮБУЮ команду в одного и того же человека: так проверяется
// дедупликация получателя, найденного через несколько команд-участников.
type sameUserPrefs struct {
	distanceByTeam map[int64]int
}

func (f *sameUserPrefs) Resolve(_ context.Context, _ domain.TenantScope, _ string, targets []notificationprefs.Target) ([]notificationprefs.Recipient, error) {
	out := make([]notificationprefs.Recipient, 0, len(targets))
	for i, t := range targets {
		out = append(out, notificationprefs.Recipient{
			Ord: i, UserID: 777, TeamID: t.TeamID, Distance: f.distanceByTeam[t.TeamID],
		})
	}
	return out, nil
}

func (*sameUserPrefs) ResolveAddressed(context.Context, domain.TenantScope, string, []int64) ([]notificationprefs.Recipient, error) {
	return nil, nil
}

func (*sameUserPrefs) ResolveTenantAdmins(context.Context, domain.TenantScope, string, []int64) ([]notificationprefs.Recipient, error) {
	return nil, nil
}

func (*sameUserPrefs) DeliveryDefaults(context.Context, domain.TenantScope) (map[string]bool, error) {
	return map[string]bool{notificationprefs.ChannelInApp: true}, nil
}
