package notify_test

import (
	"strings"
	"testing"

	"okrs/internal/core/event"
	"okrs/internal/render/notify"
)

func teamID(v int64) *int64 { return &v }

// Команде, которую добавили, текст обязан сообщать именно это — иначе руководитель
// читает «изменился состав команд» и не понимает, что цель появилась у него.
func TestRenderGoalSharedNamesTheAddedTeam(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindGoalShared, ActorName: "Анна", EntityTitle: "Снизить отток", Count: 1,
		TeamID:  teamID(7),
		Payload: map[string]any{"added_team_ids": []any{int64(7), int64(9)}},
	})
	if !strings.Contains(got.Title, "вашу команду") || !strings.Contains(got.Title, "добавил") {
		t.Errorf("заголовок должен говорить о команде получателя: %q", got.Title)
	}
	if !strings.Contains(got.Body, "Снизить отток") {
		t.Errorf("тело обязано называть цель: %q", got.Body)
	}
}

// Соседняя команда-участник читает нейтральный текст: её состава изменение не
// коснулось, но знать о нём она должна.
func TestRenderGoalSharedNeutralForOtherParticipant(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindGoalShared, ActorName: "Анна", EntityTitle: "Снизить отток", Count: 1,
		TeamID:  teamID(3),
		Payload: map[string]any{"added_team_ids": []any{int64(7)}},
	})
	if strings.Contains(got.Title, "вашу команду") {
		t.Errorf("команда 3 не менялась — текст не должен говорить о ней: %q", got.Title)
	}
	if !strings.Contains(got.Title, "состав команд") {
		t.Errorf("заголовок: %q", got.Title)
	}
}

func TestRenderGoalUnsharedNamesTheRemovedTeam(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindGoalUnshared, ActorName: "Анна", EntityTitle: "Снизить отток", Count: 1,
		TeamID:  teamID(7),
		Payload: map[string]any{"removed_team_ids": []any{int64(7)}},
	})
	if !strings.Contains(got.Title, "вашу команду") || !strings.Contains(got.Title, "убрал") {
		t.Errorf("заголовок должен говорить, что команду убрали: %q", got.Title)
	}
}

func TestRenderGoalUnsharedNeutralForRemainingParticipant(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindGoalUnshared, ActorName: "Анна", EntityTitle: "Снизить отток", Count: 1,
		TeamID:  teamID(3),
		Payload: map[string]any{"removed_team_ids": []any{int64(7)}},
	})
	if strings.Contains(got.Title, "вашу команду") {
		t.Errorf("команда 3 осталась участником — текст не должен говорить о ней: %q", got.Title)
	}
	if !strings.Contains(got.Title, "состав команд") {
		t.Errorf("заголовок: %q", got.Title)
	}
}

// Лента читает payload_json из БД, где числа приходят float64. Читатель не должен
// различать «построено в процессе» и «прочитано из ленты».
func TestRenderGoalSharedAcceptsFloatIDsFromPayloadJSON(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindGoalShared, ActorName: "Анна", EntityTitle: "Цель", Count: 1,
		TeamID:  teamID(7),
		Payload: map[string]any{"added_team_ids": []any{float64(7)}},
	})
	if !strings.Contains(got.Title, "вашу команду") {
		t.Errorf("float64 из JSONB обязан сопоставляться с TeamID так же, как int64: %q", got.Title)
	}
}

// Отсутствующий, пустой или битый список не должен ронять рендер и не должен
// заявлять получателю ничего о его команде.
func TestRenderGoalSharedWithoutPayloadIsNeutral(t *testing.T) {
	cases := map[string]notify.Input{
		"нет payload":      {Kind: event.KindGoalShared, ActorName: "Анна", EntityTitle: "Цель", Count: 1, TeamID: teamID(7)},
		"пустой список":    {Kind: event.KindGoalShared, ActorName: "Анна", EntityTitle: "Цель", Count: 1, TeamID: teamID(7), Payload: map[string]any{"added_team_ids": []any{}}},
		"чужой тип в поле": {Kind: event.KindGoalShared, ActorName: "Анна", EntityTitle: "Цель", Count: 1, TeamID: teamID(7), Payload: map[string]any{"added_team_ids": "7"}},
		"нет TeamID":       {Kind: event.KindGoalShared, ActorName: "Анна", EntityTitle: "Цель", Count: 1, Payload: map[string]any{"added_team_ids": []any{int64(7)}}},
	}
	for name, in := range cases {
		got := notify.Render(in)
		if strings.Contains(got.Title, "вашу команду") {
			t.Errorf("%s: нельзя утверждать, что менялась команда получателя: %q", name, got.Title)
		}
		if got.Title == "" || got.Title == notify.FallbackTitle {
			t.Errorf("%s: заголовок должен остаться осмысленным, got %q", name, got.Title)
		}
	}
}

// Регресс: схлопывание замораживает kind первого события и перезаписывает payload
// последним. Строка с kind=goal_shared, в payload которой лежат СНЯТЫЕ команды,
// обязана читаться как снятие — иначе команде, которую только что убрали,
// сообщат, что её добавили.
func TestRenderCompositionFollowsPayloadNotKind(t *testing.T) {
	got := notify.Render(notify.Input{
		Kind: event.KindGoalShared, ActorName: "Анна", EntityTitle: "Цель", Count: 1,
		TeamID:  teamID(7),
		Payload: map[string]any{"removed_team_ids": []any{int64(7)}},
	})
	if !strings.Contains(got.Title, "убрал") {
		t.Errorf("payload говорит о снятии — текст обязан следовать за ним: %q", got.Title)
	}
}

// Решение замечания к общей цели приходит командам участников, которые этого
// замечания не писали: называть его «вашим» нельзя.
func TestRenderResolvedCommentForParticipantsIsNotPossessive(t *testing.T) {
	team := notify.Render(notify.Input{
		Kind: event.KindCommentResolved, Type: notify.TypeGoalComment,
		ActorName: "Анна", EntityTitle: "Цель", Count: 1,
	})
	if strings.Contains(team.Title, "ваш") {
		t.Errorf("участники команд замечание не писали: %q", team.Title)
	}
	if !strings.Contains(team.Title, "решил") {
		t.Errorf("заголовок всё же обязан говорить о решении: %q", team.Title)
	}
	author := notify.Render(notify.Input{
		Kind: event.KindCommentResolved, Type: "my_comment_resolved",
		ActorName: "Анна", EntityTitle: "Цель", Count: 1,
	})
	if !strings.Contains(author.Title, "ваш") {
		t.Errorf("адресная копия автору обязана остаться прежней: %q", author.Title)
	}
}

// Снятой команде переход обязан вести на её доску, а не на отсутствующую у неё
// цель.
func TestTargetURLForRemovedTeamStopsAtTheBoard(t *testing.T) {
	team, period, goal := int64(7), int64(3), int64(10)
	got := notify.TargetURL(notify.LinkInput{
		Kind: event.KindGoalUnshared, TeamID: &team, PeriodID: &period, GoalID: &goal,
		GoalLeftTeam: true,
	})
	if strings.Contains(got, "goal=") {
		t.Errorf("цели на доске снятой команды больше нет, ссылка на неё бессмысленна: %q", got)
	}
	if !strings.Contains(got, "team=7") || !strings.Contains(got, "period=3") {
		t.Errorf("переход обязан вести на доску команды в периоде цели: %q", got)
	}
	// Оставшимся участникам ссылка не меняется.
	same := notify.TargetURL(notify.LinkInput{
		Kind: event.KindGoalUnshared, TeamID: &team, PeriodID: &period, GoalID: &goal,
	})
	if !strings.Contains(same, "goal=10") {
		t.Errorf("для оставшейся команды ссылка на цель обязана сохраниться: %q", same)
	}
}

// TeamWasRemoved — то, из чего вызывающие строят GoalLeftTeam.
func TestTeamWasRemoved(t *testing.T) {
	seven := int64(7)
	if !notify.TeamWasRemoved(map[string]any{"removed_team_ids": []any{float64(7)}}, &seven) {
		t.Error("команда в списке снятых")
	}
	if notify.TeamWasRemoved(map[string]any{"added_team_ids": []any{int64(7)}}, &seven) {
		t.Error("добавление снятием не является")
	}
	if notify.TeamWasRemoved(nil, &seven) || notify.TeamWasRemoved(map[string]any{"removed_team_ids": []any{int64(7)}}, nil) {
		t.Error("без payload или без команды ответ — false")
	}
}
