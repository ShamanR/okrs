package notification

import (
	"strings"
	"testing"
	"unicode/utf8"

	"okrs/internal/core/domain"
	"okrs/internal/core/event"
	"okrs/internal/render/notify"
)

// Комментарий не имеет ограничения длины нигде в системе, а payloadOf — единственное
// место, которое размножает его текст на N получателей (по копии payload_json на
// каждого адресата). Без обрезки один длинный комментарий на широко расшаренной цели
// усиливает неограниченный пользовательский текст N раз.
func TestPayloadOfTruncatesLongCommentText(t *testing.T) {
	long := strings.Repeat("я", payloadTextPreviewLimit+50)
	p := payloadOf(event.CommentAdded{GoalID: 1, CommentID: 2, GoalTitle: "Цель", Text: long})
	got, _ := p["text"].(string)
	if utf8.RuneCountInString(got) > payloadTextPreviewLimit+1 { // +1 допускает добавленное многоточие
		t.Fatalf("длина усечённого текста в рунах = %d, want <= %d", utf8.RuneCountInString(got), payloadTextPreviewLimit+1)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("усечённый текст должен заканчиваться многоточием: %q", got)
	}
}

// Короткий текст обязан пройти без изменений — обрезка не должна портить обычный
// случай добавлением многоточия там, где текст и так укладывается в лимит.
func TestPayloadOfLeavesShortCommentTextUntouched(t *testing.T) {
	p := payloadOf(event.ReplyAdded{GoalID: 1, CommentID: 2, GoalTitle: "Цель", Text: "коротко"})
	got, _ := p["text"].(string)
	if got != "коротко" {
		t.Errorf("text = %q, want %q без изменений", got, "коротко")
	}
}

// truncateText режет по рунам, а не по байтам: обрезка посередине многобайтового
// символа дала бы невалидный UTF-8 в payload_json.
func TestTruncateTextIsRuneSafe(t *testing.T) {
	s := strings.Repeat("駄", payloadTextPreviewLimit) + "駄駄駄"
	got := truncateText(s)
	if !utf8.ValidString(got) {
		t.Fatalf("truncateText вернул невалидный UTF-8: %q", got)
	}
	if utf8.RuneCountInString(got) != payloadTextPreviewLimit+1 {
		t.Fatalf("длина в рунах = %d, want %d (лимит + многоточие)", utf8.RuneCountInString(got), payloadTextPreviewLimit+1)
	}
}

// Текст заметки обязан обрезаться ровно так же, как текст комментария: колонка
// key_result_notes.text — TEXT без ограничения, maxlength в форме нет, серверной
// проверки нет, а payloadOf — единственное место, размножающее пользовательское поле
// по копии payload_json на каждого получателя. Заметке достаётся хуже комментария:
// CheckIn читает её безусловно, поэтому ОБЕ стороны (before и after) попадают в
// payload каждого получателя на КАЖДОМ чек-ине, включая те, где заметка не менялась.
func TestPayloadOfTruncatesBothNoteSides(t *testing.T) {
	long := strings.Repeat("я", payloadTextPreviewLimit+50)
	p := payloadOf(event.KRCheckedIn{
		GoalID: 1, KRID: 2, KRTitle: "MAU",
		NoteBefore: long, NoteAfter: long + "хвост",
	})
	for _, side := range []string{"before", "after"} {
		m, ok := p[side].(map[string]any)
		if !ok {
			t.Fatalf("%s: нет вложенной карты в payload", side)
		}
		got, _ := m["note"].(string)
		if utf8.RuneCountInString(got) > payloadTextPreviewLimit+1 { // +1 допускает добавленное многоточие
			t.Errorf("%s: длина заметки в рунах = %d, want <= %d", side, utf8.RuneCountInString(got), payloadTextPreviewLimit+1)
		}
		if !strings.HasSuffix(got, "…") {
			t.Errorf("%s: усечённая заметка должна заканчиваться многоточием, длина = %d", side, utf8.RuneCountInString(got))
		}
	}
}

// Короткая заметка обязана пройти без изменений — обрезка не должна портить обычный
// случай, а рендерер пишет её в тело дословно (правила 3 и 4).
func TestPayloadOfLeavesShortNoteUntouched(t *testing.T) {
	p := payloadOf(event.KRCheckedIn{GoalID: 1, KRID: 2, NoteAfter: "ждём поставку"})
	after, _ := p["after"].(map[string]any)
	if got, _ := after["note"].(string); got != "ждём поставку" {
		t.Errorf("note = %q, want %q", got, "ждём поставку")
	}
}

// Шов «мост → рендерер». Тесты рендерера строят payload руками, поэтому переименование
// ключа payload в payloadOf (например "health" → "health_status") проходило весь набор
// зелёным, а в проде каждый чек-ин рисовался бы с пустыми иконкой и подписью, и правила
// 2/4/5 схлопывались бы в 1/3/default. Здесь настоящий payloadOf прогоняется прямо в
// notify.Render и текст сверяется с таблицей правил — тест закрывает шов целиком, а не
// одну его сторону.
func TestPayloadOfRendersThroughNotifyPerRules(t *testing.T) {
	const (
		atRisk  = domain.KRHealthAtRisk
		onTrack = domain.KRHealthOnTrack
	)
	cases := []struct {
		name string
		ev   event.KRCheckedIn
		want string
	}{
		{
			name: "правило 1: только прогресс",
			ev:   event.KRCheckedIn{ProgressBefore: 10, ProgressAfter: 60, HealthBefore: onTrack, HealthAfter: onTrack},
			want: "●On Track 10% → 60%",
		},
		{
			name: "правило 2: прогресс и статус",
			ev:   event.KRCheckedIn{ProgressBefore: 10, ProgressAfter: 60, HealthBefore: atRisk, HealthAfter: onTrack},
			want: "▲At Risk → ●On Track 10% → 60%",
		},
		{
			name: "правило 3: только заметка",
			ev:   event.KRCheckedIn{ProgressBefore: 40, ProgressAfter: 40, HealthBefore: onTrack, HealthAfter: onTrack, NoteAfter: "ждём поставку"},
			want: "●On Track — заметка: ждём поставку",
		},
		{
			name: "правило 4: заметка и статус",
			ev:   event.KRCheckedIn{ProgressBefore: 40, ProgressAfter: 40, HealthBefore: atRisk, HealthAfter: onTrack, NoteAfter: "ждём поставку"},
			want: "▲At Risk → ●On Track — заметка: ждём поставку",
		},
		{
			name: "правило 5: только статус",
			ev:   event.KRCheckedIn{ProgressBefore: 40, ProgressAfter: 40, HealthBefore: atRisk, HealthAfter: onTrack},
			want: "▲At Risk → ●On Track",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := tc.ev
			ev.GoalID, ev.KRID, ev.KRTitle = 1, 2, "MAU"
			got := notify.Render(notify.Input{
				Kind:        ev.Kind(),
				ActorName:   "Пётр",
				EntityTitle: anchorOf(ev).title,
				Count:       1,
				Payload:     payloadOf(ev),
			})
			if got.Body != tc.want {
				t.Errorf("body = %q, want %q", got.Body, tc.want)
			}
			// Предмет уведомления идёт из того же шва: без него читатель не узнает,
			// о каком именно ключевом результате чек-ин.
			if got.Subject != "MAU" {
				t.Errorf("subject = %q, want %q", got.Subject, "MAU")
			}
		})
	}
}

// Регресс, найденный ревью: обрезка заметки в payload и решение «заметка изменилась»
// сравнением обрезанных строк несовместимы. Заметка длиннее лимита, отредактированная
// ЗА пределами лимита, обрезается с обеих сторон до двух одинаковых строк — рендерер
// решал бы, что заметка не менялась, и тело падало бы в ветку по умолчанию ("●On
// Track") вместо правила 3. Признак note_changed считается по полным строкам в
// payloadOf, поэтому правило 3 срабатывает, а обрезка при этом сохраняется.
func TestPayloadOfLongNoteEditedPastLimitStillRendersRule3(t *testing.T) {
	head := strings.Repeat("я", payloadTextPreviewLimit+100)
	ev := event.KRCheckedIn{
		GoalID: 1, KRID: 2, KRTitle: "MAU",
		ProgressBefore: 40, ProgressAfter: 40,
		HealthBefore: domain.KRHealthOnTrack,
		HealthAfter:  domain.KRHealthOnTrack,
		NoteBefore:   head + " было",
		NoteAfter:    head + " стало",
	}
	p := payloadOf(ev)

	// Предпосылка теста: обе стороны действительно обрезаны до одинакового текста —
	// иначе тест проверял бы не то, что сломалось.
	before, _ := p["before"].(map[string]any)
	after, _ := p["after"].(map[string]any)
	if before["note"] != after["note"] {
		t.Fatalf("предпосылка не выполнена: обрезанные строки должны совпадать")
	}
	if utf8.RuneCountInString(after["note"].(string)) > payloadTextPreviewLimit+1 {
		t.Fatalf("обрезка обязана сохраниться: длина = %d", utf8.RuneCountInString(after["note"].(string)))
	}

	got := notify.Render(notify.Input{
		Kind: ev.Kind(), ActorName: "Пётр", EntityTitle: anchorOf(ev).title, Count: 1, Payload: p,
	})
	if !strings.HasPrefix(got.Body, "●On Track — заметка: ") {
		t.Fatalf("правило 3 не сработало, body = %q", got.Body)
	}
}

// Очищенная заметка при неизменных прогрессе и статусе — единственный оставшийся
// реальный случай ветки по умолчанию: note_changed истинен, но пустой текст никогда
// не показывается, и висящего «— заметка:» быть не должно.
func TestPayloadOfClearedNoteFallsBackToStatusOnly(t *testing.T) {
	ev := event.KRCheckedIn{
		GoalID: 1, KRID: 2, KRTitle: "MAU",
		ProgressBefore: 40, ProgressAfter: 40,
		HealthBefore: domain.KRHealthOnTrack,
		HealthAfter:  domain.KRHealthOnTrack,
		NoteBefore:   "было",
		NoteAfter:    "",
	}
	got := notify.Render(notify.Input{
		Kind: ev.Kind(), ActorName: "Пётр", EntityTitle: anchorOf(ev).title, Count: 1, Payload: payloadOf(ev),
	})
	if got.Body != "●On Track" {
		t.Fatalf("body = %q, want %q", got.Body, "●On Track")
	}
}
