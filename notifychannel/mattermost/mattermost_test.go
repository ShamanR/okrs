package mattermost_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"okrs/notifychannel"
	"okrs/notifychannel/mattermost"
)

// fakeMM изображает Mattermost: запоминает путь каждого запроса и отданные посты.
type fakeMM struct {
	mu       sync.Mutex
	paths    []string
	auth     string
	posted   map[string]any
	posts    []string // текст каждого поста, по порядку
	emailErr int      // если не 0, резолв email отвечает этим кодом
	meErr    int      // если не 0, /api/v4/users/me отвечает этим кодом
	postErr  int      // если не 0, создание поста отвечает этим кодом
}

func (f *fakeMM) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.paths = append(f.paths, r.Method+" "+r.URL.Path)
		f.auth = r.Header.Get("Authorization")
		emailErr, meErr, postErr := f.emailErr, f.meErr, f.postErr
		f.mu.Unlock()

		switch {
		case r.URL.Path == "/api/v4/users/me":
			if meErr != 0 {
				w.WriteHeader(meErr)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "bot-1"})
		case strings.HasPrefix(r.URL.Path, "/api/v4/users/email/"):
			if emailErr != 0 {
				w.WriteHeader(emailErr)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "user-2"})
		case r.URL.Path == "/api/v4/channels/direct":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "dm-3"})
		case r.URL.Path == "/api/v4/posts":
			if postErr != 0 {
				w.WriteHeader(postErr)
				return
			}
			f.mu.Lock()
			_ = json.NewDecoder(r.Body).Decode(&f.posted)
			if msg, _ := f.posted["message"].(string); msg != "" {
				f.posts = append(f.posts, msg)
			}
			f.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func (f *fakeMM) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, p := range f.paths {
		if p == path {
			n++
		}
	}
	return n
}

func (f *fakeMM) sentPosts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.posts...)
}

// clock — управляемые часы: окно отправки измеряется ими, поэтому тест на
// «накопил и отправил по истечении окна» двигает время, а не ждёт его.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *clock {
	return &clock{t: time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)}
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// capturingHandler собирает записи лога: ошибка доставки вызывающему не
// возвращается, поэтому единственный способ её увидеть — логгер.
type capturingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *capturingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *capturingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(string) slog.Handler      { return h }

func (h *capturingHandler) texts() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, 0, len(h.records))
	for _, r := range h.records {
		var b strings.Builder
		b.WriteString(r.Message)
		r.Attrs(func(a slog.Attr) bool {
			b.WriteString(" ")
			b.WriteString(a.Key)
			b.WriteString("=")
			b.WriteString(a.Value.String())
			return true
		})
		out = append(out, b.String())
	}
	return out
}

func newSender(t *testing.T, srv *httptest.Server) notifychannel.Sender {
	t.Helper()
	s, err := mattermost.Channel().New(notifychannel.Deps{
		Settings: notifychannel.Settings{
			Values: map[string]any{"base_url": srv.URL},
			Secret: "bot-token",
		},
	})
	if err != nil {
		t.Fatalf("конструктор: %v", err)
	}
	return s
}

// senderWith собирает канал с управляемыми часами и логгером — то, что нужно
// тестам про накопление и про ошибки доставки.
func senderWith(t *testing.T, baseURL string, values map[string]any, c *clock, h slog.Handler) notifychannel.Sender {
	t.Helper()
	vals := map[string]any{"base_url": baseURL}
	for k, v := range values {
		vals[k] = v
	}
	d := notifychannel.Deps{
		Settings: notifychannel.Settings{Values: vals, Secret: "bot-token"},
	}
	if c != nil {
		d.Now = c.now
	}
	if h != nil {
		d.Logger = slog.New(h)
	}
	s, err := mattermost.Channel().New(d)
	if err != nil {
		t.Fatalf("конструктор: %v", err)
	}
	return s
}

func TestSendNowWalksTheFullDirectMessageFlow(t *testing.T) {
	f := &fakeMM{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()

	err := newSender(t, srv).SendNow(context.Background(),
		notifychannel.Target{Email: "ivan@example.com"},
		notifychannel.Message{Title: "Пётр изменил цель", Body: "Снизить отток", URL: "/?goal_id=7"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	want := []string{
		"GET /api/v4/users/me",
		"GET /api/v4/users/email/ivan@example.com",
		"POST /api/v4/channels/direct",
		"POST /api/v4/posts",
	}
	if len(f.paths) != len(want) {
		t.Fatalf("запросы: got %v, want %v", f.paths, want)
	}
	for i := range want {
		if f.paths[i] != want[i] {
			t.Fatalf("запрос %d: got %q, want %q", i, f.paths[i], want[i])
		}
	}
	if f.auth != "Bearer bot-token" {
		t.Fatalf("авторизация: got %q", f.auth)
	}
	if f.posted["channel_id"] != "dm-3" {
		t.Fatalf("пост ушёл не в прямой канал: %+v", f.posted)
	}
	msg, _ := f.posted["message"].(string)
	if !strings.Contains(msg, "Пётр изменил цель") || !strings.Contains(msg, "Снизить отток") {
		t.Fatalf("сообщение потеряло текст: %q", msg)
	}
}

// Send принимает сообщение к отправке и держит его: до закрытия окна наружу
// не уходит ничего. Это и есть причина, по которой Send не возвращает исход
// доставки — доставки ещё не было.
func TestSendHoldsUntilTheWindowCloses(t *testing.T) {
	f := &fakeMM{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	c := newClock()
	s := senderWith(t, srv.URL, map[string]any{"window_minutes": 10}, c, nil)

	for i := 0; i < 3; i++ {
		if err := s.Send(context.Background(),
			notifychannel.Target{Email: "ivan@example.com"},
			notifychannel.Message{Title: "обновление", Body: "тело"}); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	if n := f.count("POST /api/v4/posts"); n != 0 {
		t.Fatalf("до закрытия окна наружу ушло %d постов, want 0", n)
	}

	c.advance(11 * time.Minute)
	if err := s.Send(context.Background(),
		notifychannel.Target{Email: "ivan@example.com"},
		notifychannel.Message{Title: "четвёртое", Body: "тело"}); err != nil {
		t.Fatalf("send после закрытия окна: %v", err)
	}

	posts := f.sentPosts()
	if len(posts) != 1 {
		t.Fatalf("после закрытия окна ожидался один пост, got %d: %v", len(posts), posts)
	}
	if !strings.Contains(posts[0], "4 обновления") {
		t.Fatalf("заголовок не назвал число обновлений: %q", posts[0])
	}
}

// Одно накопленное обновление выглядит так же, как выглядело до появления
// накопления: лишнего заголовка со счётчиком у него нет.
func TestSingleUpdateKeepsItsPlainShape(t *testing.T) {
	f := &fakeMM{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	s := senderWith(t, srv.URL, nil, newClock(), nil)

	if err := s.Send(context.Background(),
		notifychannel.Target{Email: "ivan@example.com"},
		notifychannel.Message{Title: "Пётр изменил цель", Body: "Снизить отток"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := s.Close(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	posts := f.sentPosts()
	if len(posts) != 1 {
		t.Fatalf("ожидался один пост, got %d", len(posts))
	}
	if strings.Contains(posts[0], "обновлени") && strings.HasPrefix(posts[0], "**1") {
		t.Fatalf("одиночное обновление не должно получать счётчик: %q", posts[0])
	}
	if !strings.Contains(posts[0], "Пётр изменил цель") {
		t.Fatalf("сообщение потеряло текст: %q", posts[0])
	}
}

// Пустое окно не порождает сообщения: получателю нечего сказать.
func TestEmptyWindowSendsNothing(t *testing.T) {
	f := &fakeMM{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	s := senderWith(t, srv.URL, nil, newClock(), nil)

	if err := s.Close(context.Background()); err != nil {
		t.Fatalf("flush пустого буфера: %v", err)
	}
	if n := len(f.paths); n != 0 {
		t.Fatalf("пустое окно сделало %d запросов: %v", n, f.paths)
	}
}

// Накопленное содержит все обновления окна, а не только последнее.
func TestDigestCarriesEveryUpdate(t *testing.T) {
	f := &fakeMM{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	s := senderWith(t, srv.URL, nil, newClock(), nil)

	titles := []string{"Иван добавил замечание", "Пётр изменил цель", "Мария обновила прогресс"}
	for _, title := range titles {
		if err := s.Send(context.Background(),
			notifychannel.Target{Email: "ivan@example.com"},
			notifychannel.Message{Title: title, Body: "тело " + title, URL: "/?goal_id=7"}); err != nil {
			t.Fatalf("send %q: %v", title, err)
		}
	}
	if err := s.Close(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	posts := f.sentPosts()
	if len(posts) != 1 {
		t.Fatalf("ожидался один пост на всё окно, got %d: %v", len(posts), posts)
	}
	for _, title := range titles {
		if !strings.Contains(posts[0], title) {
			t.Fatalf("накопленное потеряло %q: %q", title, posts[0])
		}
	}
	if !strings.Contains(posts[0], "3 обновления") {
		t.Fatalf("заголовок не назвал число обновлений: %q", posts[0])
	}
}

// Накопление раздельное по получателям: каждый получает только своё.
func TestDigestIsPerRecipient(t *testing.T) {
	f := &fakeMM{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	s := senderWith(t, srv.URL, nil, newClock(), nil)

	if err := s.Send(context.Background(), notifychannel.Target{Email: "ivan@example.com"},
		notifychannel.Message{Title: "для Ивана"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := s.Send(context.Background(), notifychannel.Target{Email: "maria@example.com"},
		notifychannel.Message{Title: "для Марии"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := s.Close(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	posts := f.sentPosts()
	if len(posts) != 2 {
		t.Fatalf("ожидалось по посту на получателя, got %d: %v", len(posts), posts)
	}
	for _, p := range posts {
		if strings.Contains(p, "для Ивана") && strings.Contains(p, "для Марии") {
			t.Fatalf("обновления получателей смешались в одном посте: %q", p)
		}
	}
}

// Немедленная отправка минует накопление в обе стороны: своё сообщение шлёт
// сразу, чужое накопленное не трогает. На этом стоит кнопка «Проверить» —
// её ответ обязан описывать именно проверочное сообщение.
func TestSendNowBypassesTheBufferInBothDirections(t *testing.T) {
	f := &fakeMM{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	s := senderWith(t, srv.URL, nil, newClock(), nil)

	if err := s.Send(context.Background(), notifychannel.Target{Email: "ivan@example.com"},
		notifychannel.Message{Title: "накопленное"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := s.SendNow(context.Background(), notifychannel.Target{Email: "admin@example.com"},
		notifychannel.Message{Title: "проверочное"}); err != nil {
		t.Fatalf("sendNow: %v", err)
	}

	posts := f.sentPosts()
	if len(posts) != 1 {
		t.Fatalf("немедленная отправка обязана дать ровно один пост, got %d: %v", len(posts), posts)
	}
	if !strings.Contains(posts[0], "проверочное") || strings.Contains(posts[0], "накопленное") {
		t.Fatalf("немедленная отправка захватила накопленное: %q", posts[0])
	}

	// Накопленное осталось на месте и уходит своим чередом.
	if err := s.Close(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}
	posts = f.sentPosts()
	if len(posts) != 2 || !strings.Contains(posts[1], "накопленное") {
		t.Fatalf("накопленное не пережило немедленную отправку: %v", posts)
	}
}

// Выгрузка опустошает буфер: повторный вызов ничего не шлёт.
func TestFlushEmptiesTheBuffer(t *testing.T) {
	f := &fakeMM{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	s := senderWith(t, srv.URL, nil, newClock(), nil)

	if err := s.Send(context.Background(), notifychannel.Target{Email: "ivan@example.com"},
		notifychannel.Message{Title: "t"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := s.Close(context.Background()); err != nil {
		t.Fatalf("первая выгрузка: %v", err)
	}
	before := len(f.sentPosts())
	if err := s.Close(context.Background()); err != nil {
		t.Fatalf("повторная выгрузка: %v", err)
	}
	if after := len(f.sentPosts()); after != before {
		t.Fatalf("повторная выгрузка отправила лишнее: было %d, стало %d", before, after)
	}
}

// Потолок накопленного — на получателя: шумный получатель не вытесняет чужое,
// а у себя теряет самое старое, а не самое свежее.
func TestBufferCapDropsOldestPerRecipient(t *testing.T) {
	f := &fakeMM{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	h := &capturingHandler{}
	s := senderWith(t, srv.URL, nil, newClock(), h)

	const overflow = 260 // потолок 200
	for i := 0; i < overflow; i++ {
		if err := s.Send(context.Background(), notifychannel.Target{Email: "noisy@example.com"},
			notifychannel.Message{Title: "шум", Body: strings.Repeat("x", 1)}); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	// Последнее обновление шумного и единственное обновление тихого.
	if err := s.Send(context.Background(), notifychannel.Target{Email: "noisy@example.com"},
		notifychannel.Message{Title: "самое свежее"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := s.Send(context.Background(), notifychannel.Target{Email: "quiet@example.com"},
		notifychannel.Message{Title: "тихое обновление"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := s.Close(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	var noisy, quiet string
	for _, p := range f.sentPosts() {
		switch {
		case strings.Contains(p, "тихое обновление"):
			quiet = p
		default:
			noisy = p
		}
	}
	if quiet == "" {
		t.Fatal("накопленное тихого получателя вытеснено шумным")
	}
	if !strings.Contains(noisy, "самое свежее") {
		t.Fatalf("отброшено самое свежее вместо самого старого: %q", noisy[:min(len(noisy), 200)])
	}
	if !strings.Contains(noisy, "200 обновлений") {
		t.Fatalf("накопленное не ограничено потолком: %q", noisy[:min(len(noisy), 200)])
	}
	if !containsText(h.texts(), "отброшена по достижении предела") {
		t.Fatalf("отбрасывание не попало в журнал: %v", h.texts())
	}
}

// Ошибка доставки накопленного вызывающему не возвращается — возвращать её
// некому — и обязана попасть в журнал.
func TestDeliveryFailureGoesToTheLogNotTheCaller(t *testing.T) {
	f := &fakeMM{emailErr: http.StatusNotFound}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	c := newClock()
	h := &capturingHandler{}
	s := senderWith(t, srv.URL, map[string]any{"window_minutes": 10}, c, h)

	if err := s.Send(context.Background(), notifychannel.Target{Email: "nobody@example.com"},
		notifychannel.Message{Title: "t"}); err != nil {
		t.Fatalf("Send обязан принять сообщение, а не вернуть исход доставки: %v", err)
	}
	c.advance(11 * time.Minute)
	if err := s.Send(context.Background(), notifychannel.Target{Email: "nobody@example.com"},
		notifychannel.Message{Title: "t2"}); err != nil {
		t.Fatalf("Send обязан принять сообщение даже при провале доставки: %v", err)
	}

	if !containsText(h.texts(), "доставка накопленного отклонена") {
		t.Fatalf("провал доставки не попал в журнал: %v", h.texts())
	}
	for _, text := range h.texts() {
		if strings.Contains(text, "nobody@example.com") {
			t.Fatalf("адрес получателя попал в журнал: %s", text)
		}
	}
}

// Временный отказ не стоит получателю его обновлений: они остаются до
// следующего окна. Постоянный отказ повторять бессмысленно — он отбрасывается.
func TestTransientFailureKeepsUpdatesPermanentDiscardsThem(t *testing.T) {
	t.Run("временный отказ сохраняет обновления", func(t *testing.T) {
		f := &fakeMM{postErr: http.StatusInternalServerError}
		srv := httptest.NewServer(f.handler())
		defer srv.Close()
		c := newClock()
		s := senderWith(t, srv.URL, nil, c, &capturingHandler{})
		ivan := notifychannel.Target{Email: "ivan@example.com"}

		if err := s.Send(context.Background(), ivan, notifychannel.Message{Title: "важное"}); err != nil {
			t.Fatalf("send: %v", err)
		}
		// Окно закрывается штатно — это и есть путь, на котором удержание живёт.
		// Доставка падает с 5xx, обновление обязано остаться.
		c.advance(11 * time.Minute)
		if err := s.Send(context.Background(), ivan, notifychannel.Message{Title: "второе"}); err != nil {
			t.Fatalf("send: %v", err)
		}
		if posts := f.sentPosts(); len(posts) != 0 {
			t.Fatalf("при 5xx ничего не должно было уйти: %v", posts)
		}

		// Сервис поднялся — следующее окно отдаёт удержанное.
		f.mu.Lock()
		f.postErr = 0
		f.mu.Unlock()
		c.advance(11 * time.Minute)
		if err := s.Send(context.Background(), ivan, notifychannel.Message{Title: "третье"}); err != nil {
			t.Fatalf("send: %v", err)
		}
		if posts := f.sentPosts(); len(posts) != 1 || !strings.Contains(posts[0], "важное") {
			t.Fatalf("обновление не пережило временный отказ: %v", posts)
		}
	})

	t.Run("постоянный отказ отбрасывает обновления", func(t *testing.T) {
		f := &fakeMM{emailErr: http.StatusNotFound}
		srv := httptest.NewServer(f.handler())
		defer srv.Close()
		s := senderWith(t, srv.URL, nil, newClock(), &capturingHandler{})

		if err := s.Send(context.Background(), notifychannel.Target{Email: "nobody@example.com"},
			notifychannel.Message{Title: "t"}); err != nil {
			t.Fatalf("send: %v", err)
		}
		if err := s.Close(context.Background()); err == nil {
			t.Fatal("выгрузка при 404 обязана вернуть ошибку")
		}

		f.mu.Lock()
		f.emailErr = 0
		f.mu.Unlock()
		if err := s.Close(context.Background()); err != nil {
			t.Fatalf("повторная выгрузка: %v", err)
		}
		if posts := f.sentPosts(); len(posts) != 0 {
			t.Fatalf("постоянно отклонённое обновление не должно повторяться: %v", posts)
		}
	})
}

// Окно отправки — поле дескриптора со значением по умолчанию. Недопустимое
// значение отвергается конструктором, чтобы ядро показало это администратору
// как ошибку конфигурации, а не как сюрприз во время доставки.
func TestWindowIsAConfigurableDescriptorField(t *testing.T) {
	var found bool
	for _, f := range mattermost.Channel().Descriptor.Fields {
		if f.Key == "window_minutes" {
			found = true
			if f.Required {
				t.Error("окно обязано быть необязательным: у него есть значение по умолчанию")
			}
		}
	}
	if !found {
		t.Fatalf("поле окна отсутствует в дескрипторе: %+v", mattermost.Channel().Descriptor.Fields)
	}
}

func TestWindowRejectsNonPositiveAndNonNumeric(t *testing.T) {
	bad := []any{0, -5, "0", "-1", "десять", 2.5, true}
	for _, v := range bad {
		if _, err := mattermost.Channel().New(notifychannel.Deps{
			Settings: notifychannel.Settings{
				Values: map[string]any{"base_url": "https://x", "window_minutes": v},
				Secret: "t",
			},
		}); err == nil {
			t.Errorf("окно %#v (%T) должно быть отвергнуто", v, v)
		}
	}

	ok := []any{nil, "", 1, 10, float64(5), "7"}
	for _, v := range ok {
		if _, err := mattermost.Channel().New(notifychannel.Deps{
			Settings: notifychannel.Settings{
				Values: map[string]any{"base_url": "https://x", "window_minutes": v},
				Secret: "t",
			},
		}); err != nil {
			t.Errorf("окно %#v (%T) должно быть принято: %v", v, v, err)
		}
	}
}

// Окно, не заданное администратором, равно десяти минутам: до девятой минуты
// накопленное держится, после одиннадцатой уходит.
func TestWindowDefaultsToTenMinutes(t *testing.T) {
	f := &fakeMM{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	c := newClock()
	s := senderWith(t, srv.URL, nil, c, nil)

	send := func(title string) {
		t.Helper()
		if err := s.Send(context.Background(), notifychannel.Target{Email: "ivan@example.com"},
			notifychannel.Message{Title: title}); err != nil {
			t.Fatalf("send %q: %v", title, err)
		}
	}

	send("первое")
	c.advance(9 * time.Minute)
	send("второе")
	if n := f.count("POST /api/v4/posts"); n != 0 {
		t.Fatalf("на девятой минуте накопленное уже ушло (%d постов)", n)
	}
	c.advance(2 * time.Minute)
	send("третье")
	if n := f.count("POST /api/v4/posts"); n != 1 {
		t.Fatalf("после одиннадцатой минуты ожидался один пост, got %d", n)
	}
}

// Идентификатор бота запрашивается один раз и переиспользуется: доставка идёт
// пачками, и лишний вызов на каждое сообщение — это N+1 по сети.
func TestBotIDIsFetchedOnce(t *testing.T) {
	f := &fakeMM{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	s := newSender(t, srv)

	for i := 0; i < 3; i++ {
		if err := s.SendNow(context.Background(),
			notifychannel.Target{Email: "ivan@example.com"},
			notifychannel.Message{Title: "t", Body: "b"}); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	if meCalls := f.count("GET /api/v4/users/me"); meCalls != 1 {
		t.Fatalf("users/me вызван %d раз, want 1", meCalls)
	}
}

// Ненайденный адресат — отдельный класс ошибки: доставка не должна ретраиться
// вечно из-за того, что у человека нет аккаунта в Mattermost.
func TestUnknownEmailIsPermanent(t *testing.T) {
	f := &fakeMM{emailErr: http.StatusNotFound}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()

	err := newSender(t, srv).SendNow(context.Background(),
		notifychannel.Target{Email: "nobody@example.com"},
		notifychannel.Message{Title: "t", Body: "b"})
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if !mattermost.IsPermanent(err) {
		t.Fatalf("ошибка должна быть помечена постоянной: %v", err)
	}
}

// Временная ошибка сервера постоянной не считается — её надо ретраить.
func TestServerErrorIsTransient(t *testing.T) {
	f := &fakeMM{emailErr: http.StatusInternalServerError}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()

	err := newSender(t, srv).SendNow(context.Background(),
		notifychannel.Target{Email: "ivan@example.com"},
		notifychannel.Message{Title: "t", Body: "b"})
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if mattermost.IsPermanent(err) {
		t.Fatalf("5xx не должна считаться постоянной: %v", err)
	}
}

// Без адреса отправлять некуда: канал адресуется по email и не реализует Linker.
func TestEmptyEmailIsPermanent(t *testing.T) {
	f := &fakeMM{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()

	err := newSender(t, srv).SendNow(context.Background(),
		notifychannel.Target{}, notifychannel.Message{Title: "t"})
	if err == nil || !mattermost.IsPermanent(err) {
		t.Fatalf("пустой email должен давать постоянную ошибку, got %v", err)
	}
}

func TestConstructorRequiresBaseURLAndSecret(t *testing.T) {
	if _, err := mattermost.Channel().New(notifychannel.Deps{
		Settings: notifychannel.Settings{Secret: "t"},
	}); err == nil {
		t.Fatal("без base_url конструктор должен отказать")
	}
	if _, err := mattermost.Channel().New(notifychannel.Deps{
		Settings: notifychannel.Settings{Values: map[string]any{"base_url": "https://x"}},
	}); err == nil {
		t.Fatal("без секрета конструктор должен отказать")
	}
}

// Дескриптор — то, из чего админка рисует форму. Она не знает про Mattermost,
// поэтому поля и признак секретного поля обязаны быть заполнены здесь.
func TestDescriptorDrivesTheAdminForm(t *testing.T) {
	d := mattermost.Channel().Descriptor
	if d.Name != "mattermost" || d.SecretField != "token" {
		t.Fatalf("дескриптор: %+v", d)
	}
	var hasURL, hasSecret bool
	for _, f := range d.Fields {
		if f.Key == "base_url" && f.Kind == notifychannel.FieldURL && f.Required {
			hasURL = true
		}
		if f.Key == "token" && f.Kind == notifychannel.FieldSecret && f.Required {
			hasSecret = true
		}
	}
	if !hasURL || !hasSecret {
		t.Fatalf("форма неполна: %+v", d.Fields)
	}
}

// Кэш botID запоминает только УСПЕХ: если первый вызов /api/v4/users/me вернул
// временную ошибку (5xx), следующая отправка должна повторить попытку.
func TestBotIDRetryAfterTransientError(t *testing.T) {
	callCount := 0
	var mu sync.Mutex
	userMeHandler := func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		n := callCount
		mu.Unlock()
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "bot-1"})
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/users/me", userMeHandler)
	mux.HandleFunc("/api/v4/users/email/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "user-2"})
	})
	mux.HandleFunc("/api/v4/channels/direct", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "dm-3"})
	})
	mux.HandleFunc("/api/v4/posts", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	srv2 := httptest.NewServer(mux)
	defer srv2.Close()

	s := senderWith(t, srv2.URL, nil, newClock(), nil)

	err1 := s.SendNow(context.Background(),
		notifychannel.Target{Email: "ivan@example.com"},
		notifychannel.Message{Title: "первая", Body: "попытка"})
	if err1 == nil {
		t.Fatal("первая отправка должна была вернуть ошибку")
	}
	if mattermost.IsPermanent(err1) {
		t.Fatalf("первая ошибка должна быть временной (5xx): %v", err1)
	}

	err2 := s.SendNow(context.Background(),
		notifychannel.Target{Email: "ivan@example.com"},
		notifychannel.Message{Title: "вторая", Body: "попытка"})
	if err2 != nil {
		t.Fatalf("вторая отправка должна была преуспеть, но вернула: %v", err2)
	}

	mu.Lock()
	defer mu.Unlock()
	if callCount != 2 {
		t.Fatalf("/api/v4/users/me вызван %d раз, want 2", callCount)
	}
}

// base_url должен использовать http или https; другие схемы (ftp, etc) отвергаются.
func TestBaseURLMustHaveHTTPOrHTTPSScheme(t *testing.T) {
	tests := []struct {
		url    string
		wantOK bool
	}{
		{"http://mattermost.example.com", true},
		{"https://mattermost.example.com", true},
		{"ftp://mattermost.example.com", false},
		{"gopher://mattermost.example.com", false},
		{"://mattermost.example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			_, err := mattermost.Channel().New(notifychannel.Deps{
				Settings: notifychannel.Settings{
					Values: map[string]any{"base_url": tt.url},
					Secret: "token",
				},
			})
			if tt.wantOK && err != nil {
				t.Fatalf("конструктор должен был принять %q, но отказал: %v", tt.url, err)
			}
			if !tt.wantOK && err == nil {
				t.Fatalf("конструктор должен был отказать %q", tt.url)
			}
		})
	}
}

// Множественные отправки на непрогретом sender с успешным резолвом должны
// коалесцировать: первая горутина резолвит botID, остальные ждут и
// переиспользуют результат. /api/v4/users/me должен быть вызван ровно один
// раз — это требование про N+1.
func TestBotIDCoalescesOnSuccess(t *testing.T) {
	f := &fakeMM{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	s := senderWith(t, srv.URL, nil, newClock(), nil)

	const numGoroutines = 20
	start := make(chan struct{})
	done := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			<-start // Wait for signal to start simultaneously
			done <- s.SendNow(context.Background(),
				notifychannel.Target{Email: "ivan@example.com"},
				notifychannel.Message{Title: "t", Body: "b"})
		}()
	}

	close(start) // Signal all goroutines to start simultaneously

	for i := 0; i < numGoroutines; i++ {
		if err := <-done; err != nil {
			t.Fatalf("SendNow %d: %v", i, err)
		}
	}

	if meCalls := f.count("GET /api/v4/users/me"); meCalls != 1 {
		t.Fatalf("/api/v4/users/me вызван %d раз, want 1", meCalls)
	}
}

// Отказ волны резолва бота (сам /api/v4/users/me отвечает 500) должен коалесцироваться
// так же, как и успех: все ожидающие получают ОДИН и тот же исход волны параллельно,
// а не выстраиваются в очередь по одному, каждый со своим сетевым запросом.
// Именно этот сценарий был предметом бага раунда 3 (последовательная очередь по 15с);
// эмейл-эндпоинт тут ни при чём — до него в такой волне дело вообще не доходит.
func TestBotIDAllWaitOnSingleFailure(t *testing.T) {
	f := &fakeMM{meErr: http.StatusInternalServerError}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	s := senderWith(t, srv.URL, nil, newClock(), nil)

	const numGoroutines = 20
	start := make(chan struct{})
	done := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			<-start // барьер: все горутины стартуют одновременно, без time.Sleep
			done <- s.SendNow(context.Background(),
				notifychannel.Target{Email: "ivan@example.com"},
				notifychannel.Message{Title: "t", Body: "b"})
		}()
	}

	startTime := time.Now()
	close(start)

	// Все горутины обязаны получить ошибку — и получить её быстро, параллельно,
	// а не по очереди с 15-секундным http.Client.Timeout на каждую попытку.
	for i := 0; i < numGoroutines; i++ {
		err := <-done
		if err == nil {
			t.Fatal("ожидалась ошибка")
		}
		if !strings.Contains(err.Error(), "status 500") {
			t.Fatalf("ошибка должна содержать сведения о 500: %v", err)
		}
	}

	elapsed := time.Since(startTime)
	// Если бы коалесинга отказа не было, каждый ожидающий делал бы собственный
	// запрос последовательно: 20 попыток при таймауте 15с — это 300 секунд.
	// Параллельный путь укладывается в доли секунды даже на медленной машине.
	if elapsed > 2*time.Second {
		t.Fatalf("все горутины завершились за %v, слишком долго (похоже на последовательную очередь)", elapsed)
	}

	// Ровно один сетевой запрос на всю волну — это и есть коалесинг отказа.
	if meCalls := f.count("GET /api/v4/users/me"); meCalls != 1 {
		t.Fatalf("/api/v4/users/me вызван %d раз, want 1", meCalls)
	}
}

// Отмена контекста должна выигрывать гонку с ожиданием волны: даже если волна идёт,
// вызывающий с отменённым контекстом выходит сразу с ошибкой контекста.
func TestBotIDCancellationIsRespected(t *testing.T) {
	// Создам slow server, чтобы волна висела достаточно долго
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/users/me" {
			time.Sleep(2 * time.Second) // Hang the response
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "bot-1"})
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer slowServer.Close()

	s := senderWith(t, slowServer.URL, nil, newClock(), nil)

	// Start one goroutine that will hang waiting for the slow wave
	start := make(chan struct{})
	slowDone := make(chan error)

	go func() {
		<-start
		// This will initiate the wave
		slowDone <- s.SendNow(context.Background(),
			notifychannel.Target{Email: "ivan@example.com"},
			notifychannel.Message{Title: "t", Body: "b"})
	}()

	// Start one goroutine with a short timeout that joins the wave
	timeoutDone := make(chan error)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		<-start
		timeoutDone <- s.SendNow(ctx,
			notifychannel.Target{Email: "ivan@example.com"},
			notifychannel.Message{Title: "t", Body: "b"})
	}()

	// Start both simultaneously
	close(start)

	// The timeout one should complete quickly with context error
	select {
	case err := <-timeoutDone:
		if err == nil {
			t.Fatal("ожидалась ошибка контекста")
		}
		if !strings.Contains(err.Error(), "context") {
			t.Fatalf("ошибка должна упоминать контекст: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for context cancellation to take effect")
	}

	// The slow one can finish whenever (we're not waiting for it)
	<-slowDone
}

// Транспортный отказ обязан оставаться распознаваемым через net.Error: ядро
// (хендлер проверочной отправки) именно так отличает «не достучались до сервера
// канала» от «сервер канала ответил отказом» и подменяет текст первого общим
// сообщением, чтобы кнопка «Проверить» не стала сканером внутренней сети с
// оракулом. Если канал когда-нибудь свернёт ошибку через %v, эта связка молча
// развалится — тест держит её со стороны канала.
func TestTransportFailureStaysRecognisableAsNetError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // порт закрыт: следующий запрос упрётся в connection refused

	s := senderWith(t, url, nil, newClock(), nil)
	sendErr := s.SendNow(context.Background(), notifychannel.Target{Email: "a@example.com"},
		notifychannel.Message{Title: "t"})
	if sendErr == nil {
		t.Fatal("отправка на закрытый порт обязана падать")
	}
	var ne net.Error
	if !errors.As(sendErr, &ne) {
		t.Fatalf("транспортная ошибка потеряла net.Error в цепочке: %v", sendErr)
	}
	// И заодно фиксируем, ради чего всё: сырой текст содержит адрес.
	if !strings.Contains(sendErr.Error(), "127.0.0.1") {
		t.Fatalf("ожидался адрес в сыром тексте ошибки: %v", sendErr)
	}
}

// Ошибка канала уходит вызывающему и попадает в лог приложения, поэтому адрес
// получателя в её тексте оказаться не должен. Отлавливать его из готового текста
// шаблоном ненадёжно: принимаемые формы адреса шире любого разумного шаблона —
// однобуквенный TLD, интернационализированные домены. Поэтому адрес просто
// не попадает в сообщение.
func TestErrorTextNeverCarriesTheAddress(t *testing.T) {
	addresses := []string{
		"nobody@example.com",
		"a@b.c",
		"почта@пример.рф",
	}
	for _, addr := range addresses {
		t.Run(addr, func(t *testing.T) {
			f := &fakeMM{emailErr: http.StatusNotFound}
			srv := httptest.NewServer(f.handler())
			defer srv.Close()

			err := newSender(t, srv).SendNow(context.Background(),
				notifychannel.Target{Email: addr},
				notifychannel.Message{Title: "t", Body: "b"})
			if err == nil {
				t.Fatal("ожидалась ошибка")
			}

			text := err.Error()
			if strings.Contains(text, addr) || strings.Contains(text, neturl.PathEscape(addr)) {
				t.Fatalf("адрес попал в текст ошибки: %s", text)
			}
			// Диагностика при этом сохраняется: видно, какая ручка и какой статус.
			if !strings.Contains(text, "users/email") || !strings.Contains(text, "404") {
				t.Errorf("ошибка потеряла диагностику: %s", text)
			}
		})
	}
}

func containsText(texts []string, want string) bool {
	for _, t := range texts {
		if strings.Contains(t, want) {
			return true
		}
	}
	return false
}

// Ссылка уходит разметкой, а не голым адресом: получателю нужен кликабельный
// текст, а не строка запроса. Ровно то, чего не хватало в первом же реально
// доставленном сообщении.
func TestMessageRendersTheLinkAsAClickableMarkdownLink(t *testing.T) {
	f := &fakeMM{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()

	const url = "https://okr.example.com/?team=13&period=2&goal=72&comment=615"
	err := newSender(t, srv).SendNow(context.Background(),
		notifychannel.Target{Email: "ivan@example.com"},
		notifychannel.Message{Title: "Пётр изменил цель", Body: "Прозрачность", URL: url})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	posts := f.sentPosts()
	if len(posts) != 1 {
		t.Fatalf("постов: %d", len(posts))
	}
	if !strings.Contains(posts[0], "]("+url+")") {
		t.Fatalf("ссылка ушла без разметки: %q", posts[0])
	}
	if strings.Contains(posts[0], "\n"+url) {
		t.Fatalf("ссылка ушла голым адресом: %q", posts[0])
	}
}

// Сообщение без ссылки не должно получать пустую разметку.
func TestMessageWithoutURLHasNoLinkMarkup(t *testing.T) {
	f := &fakeMM{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()

	if err := newSender(t, srv).SendNow(context.Background(),
		notifychannel.Target{Email: "ivan@example.com"},
		notifychannel.Message{Title: "Заголовок", Body: "Тело"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if posts := f.sentPosts(); strings.Contains(posts[0], "](") {
		t.Fatalf("появилась разметка ссылки при отсутствии ссылки: %q", posts[0])
	}
}

// Закрытие при временном отказе обновления НЕ удерживает и окно заново НЕ
// открывает.
//
// Это то, чем закрытие отличается от закрытия окна. Ядро закрывает экземпляр,
// когда настройки, на которых он собран, уже отменены: администратор сменил
// токен или выключил канал. Удержание в этот момент означало бы, что снятый
// экземпляр ставит себе новый таймер и спустя окно постит по отозванным
// настройкам — ровно то, ради устранения чего его и снимали.
func TestCloseDoesNotRetainOrRearmOnTransientFailure(t *testing.T) {
	f := &fakeMM{postErr: http.StatusInternalServerError}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	c := newClock()
	s := senderWith(t, srv.URL, nil, c, &capturingHandler{})
	ivan := notifychannel.Target{Email: "ivan@example.com"}

	if err := s.Send(context.Background(), ivan, notifychannel.Message{Title: "важное"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := s.Close(context.Background()); err == nil {
		t.Fatal("закрытие при 5xx обязано вернуть ошибку")
	}

	// Сервис поднялся, и времени прошло больше окна. Если бы закрытие удержало
	// обновление и открыло окно, оно ушло бы сейчас — по настройкам, которых
	// уже нет.
	f.mu.Lock()
	f.postErr = 0
	f.mu.Unlock()
	c.advance(11 * time.Minute)
	if err := s.Close(context.Background()); err != nil {
		t.Fatalf("повторное закрытие: %v", err)
	}
	if posts := f.sentPosts(); len(posts) != 0 {
		t.Fatalf("закрытый экземпляр отправил удержанное: %v", posts)
	}
}

// Закрытый экземпляр больше ничего не принимает — ни в окно, ни немедленно.
func TestClosedSenderAcceptsNothingFurther(t *testing.T) {
	f := &fakeMM{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	s := senderWith(t, srv.URL, nil, newClock(), &capturingHandler{})
	ivan := notifychannel.Target{Email: "ivan@example.com"}

	if err := s.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := s.Send(context.Background(), ivan, notifychannel.Message{Title: "после"}); !errors.Is(err, mattermost.ErrClosed) {
		t.Fatalf("закрытый экземпляр принял сообщение: %v", err)
	}
	if err := s.SendNow(context.Background(), ivan, notifychannel.Message{Title: "после"}); !errors.Is(err, mattermost.ErrClosed) {
		t.Fatalf("закрытый экземпляр выполнил немедленную отправку: %v", err)
	}
	if posts := f.sentPosts(); len(posts) != 0 {
		t.Fatalf("после закрытия что-то ушло: %v", posts)
	}
}
