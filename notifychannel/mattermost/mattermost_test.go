package mattermost_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"okrs/notifychannel"
	"okrs/notifychannel/mattermost"
)

// fakeMM изображает Mattermost: запоминает путь каждого запроса и отданный пост.
type fakeMM struct {
	mu       sync.Mutex
	paths    []string
	auth     string
	posted   map[string]any
	emailErr int // если не 0, резолв email отвечает этим кодом
	meErr    int // если не 0, /api/v4/users/me отвечает этим кодом
}

func (f *fakeMM) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.paths = append(f.paths, r.Method+" "+r.URL.Path)
		f.auth = r.Header.Get("Authorization")
		f.mu.Unlock()

		switch {
		case r.URL.Path == "/api/v4/users/me":
			if f.meErr != 0 {
				w.WriteHeader(f.meErr)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "bot-1"})
		case strings.HasPrefix(r.URL.Path, "/api/v4/users/email/"):
			if f.emailErr != 0 {
				w.WriteHeader(f.emailErr)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "user-2"})
		case r.URL.Path == "/api/v4/channels/direct":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "dm-3"})
		case r.URL.Path == "/api/v4/posts":
			f.mu.Lock()
			_ = json.NewDecoder(r.Body).Decode(&f.posted)
			f.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func newSender(t *testing.T, srv *httptest.Server) notifychannel.Sender {
	t.Helper()
	s, err := mattermost.Channel().New(notifychannel.Settings{
		Values: map[string]any{"base_url": srv.URL},
		Secret: "bot-token",
	})
	if err != nil {
		t.Fatalf("конструктор: %v", err)
	}
	return s
}

func TestSendWalksTheFullDirectMessageFlow(t *testing.T) {
	f := &fakeMM{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()

	err := newSender(t, srv).Send(context.Background(),
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

// Идентификатор бота запрашивается один раз и переиспользуется: воркер доставки
// шлёт пачками, и лишний вызов на каждое сообщение — это N+1 по сети.
func TestBotIDIsFetchedOnce(t *testing.T) {
	f := &fakeMM{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	s := newSender(t, srv)

	for i := 0; i < 3; i++ {
		if err := s.Send(context.Background(),
			notifychannel.Target{Email: "ivan@example.com"},
			notifychannel.Message{Title: "t", Body: "b"}); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	var meCalls int
	for _, p := range f.paths {
		if p == "GET /api/v4/users/me" {
			meCalls++
		}
	}
	if meCalls != 1 {
		t.Fatalf("users/me вызван %d раз, want 1", meCalls)
	}
}

// Ненайденный адресат — отдельный класс ошибки: доставка не должна ретраиться
// вечно из-за того, что у человека нет аккаунта в Mattermost.
func TestUnknownEmailIsPermanent(t *testing.T) {
	f := &fakeMM{emailErr: http.StatusNotFound}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()

	err := newSender(t, srv).Send(context.Background(),
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

	err := newSender(t, srv).Send(context.Background(),
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

	err := newSender(t, srv).Send(context.Background(),
		notifychannel.Target{}, notifychannel.Message{Title: "t"})
	if err == nil || !mattermost.IsPermanent(err) {
		t.Fatalf("пустой email должен давать постоянную ошибку, got %v", err)
	}
}

func TestConstructorRequiresBaseURLAndSecret(t *testing.T) {
	if _, err := mattermost.Channel().New(notifychannel.Settings{Secret: "t"}); err == nil {
		t.Fatal("без base_url конструктор должен отказать")
	}
	if _, err := mattermost.Channel().New(notifychannel.Settings{
		Values: map[string]any{"base_url": "https://x"},
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
// временную ошибку (5xx), следующий Send() должен переопубликовать попытку.
func TestBotIDRetryAfterTransientError(t *testing.T) {
	f := &fakeMM{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()

	// Первый обработчик ошибка, второй успех
	callCount := 0
	userMeHandler := func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
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

	s, err := mattermost.Channel().New(notifychannel.Settings{
		Values: map[string]any{"base_url": srv2.URL},
		Secret: "bot-token",
	})
	if err != nil {
		t.Fatalf("конструктор: %v", err)
	}

	// Первый Send() должен упасть с временной ошибкой
	err1 := s.Send(context.Background(),
		notifychannel.Target{Email: "ivan@example.com"},
		notifychannel.Message{Title: "первая", Body: "попытка"})
	if err1 == nil {
		t.Fatal("первый Send() должен был вернуть ошибку")
	}
	if mattermost.IsPermanent(err1) {
		t.Fatalf("первая ошибка должна быть временной (5xx): %v", err1)
	}

	// Второй Send() должен переопубликовать запрос к /api/v4/users/me и преуспеть
	err2 := s.Send(context.Background(),
		notifychannel.Target{Email: "ivan@example.com"},
		notifychannel.Message{Title: "вторая", Body: "попытка"})
	if err2 != nil {
		t.Fatalf("второй Send() должен был преуспеть, но вернул: %v", err2)
	}

	// /api/v4/users/me должен быть вызван дважды: один раз (500), второй раз (успех)
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
			_, err := mattermost.Channel().New(notifychannel.Settings{
				Values: map[string]any{"base_url": tt.url},
				Secret: "token",
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

// Множественные Send() на непрогретом sender с успешным резолвом должны коалесцировать:
// первая горутина резолвит botID, остальные ждут и переиспользуют результат.
// /api/v4/users/me должен быть вызван ровно один раз — это требование про N+1.
func TestBotIDCoalescesOnSuccess(t *testing.T) {
	f := &fakeMM{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()

	s, err := mattermost.Channel().New(notifychannel.Settings{
		Values: map[string]any{"base_url": srv.URL},
		Secret: "bot-token",
	})
	if err != nil {
		t.Fatalf("конструктор: %v", err)
	}

	const numGoroutines = 20
	start := make(chan struct{})
	done := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			<-start // Wait for signal to start simultaneously
			err := s.Send(context.Background(),
				notifychannel.Target{Email: "ivan@example.com"},
				notifychannel.Message{Title: "t", Body: "b"})
			done <- err
		}(i)
	}

	close(start) // Signal all goroutines to start simultaneously

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		if err := <-done; err != nil {
			t.Fatalf("Send %d: %v", i, err)
		}
	}

	// Count /api/v4/users/me calls
	var meCalls int
	for _, p := range f.paths {
		if p == "GET /api/v4/users/me" {
			meCalls++
		}
	}
	if meCalls != 1 {
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

	s, err := mattermost.Channel().New(notifychannel.Settings{
		Values: map[string]any{"base_url": srv.URL},
		Secret: "bot-token",
	})
	if err != nil {
		t.Fatalf("конструктор: %v", err)
	}

	const numGoroutines = 20
	start := make(chan struct{})
	done := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			<-start // барьер: все горутины стартуют одновременно, без time.Sleep
			err := s.Send(context.Background(),
				notifychannel.Target{Email: "ivan@example.com"},
				notifychannel.Message{Title: "t", Body: "b"})
			done <- err
		}(i)
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
	f.mu.Lock()
	var meCalls int
	for _, p := range f.paths {
		if p == "GET /api/v4/users/me" {
			meCalls++
		}
	}
	f.mu.Unlock()
	if meCalls != 1 {
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

	s, err := mattermost.Channel().New(notifychannel.Settings{
		Values: map[string]any{"base_url": slowServer.URL},
		Secret: "bot-token",
	})
	if err != nil {
		t.Fatalf("конструктор: %v", err)
	}

	// Start one goroutine that will hang waiting for the slow wave
	start := make(chan struct{})
	slowDone := make(chan error)

	go func() {
		<-start
		// This will initiate the wave
		err := s.Send(context.Background(),
			notifychannel.Target{Email: "ivan@example.com"},
			notifychannel.Message{Title: "t", Body: "b"})
		slowDone <- err
	}()

	// Start one goroutine with a short timeout that joins the wave
	timeoutDone := make(chan error)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		<-start
		err := s.Send(ctx,
			notifychannel.Target{Email: "ivan@example.com"},
			notifychannel.Message{Title: "t", Body: "b"})
		timeoutDone <- err
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

	s, err := mattermost.Channel().New(notifychannel.Settings{
		Values: map[string]any{"base_url": url},
		Secret: "bot-token",
	})
	if err != nil {
		t.Fatalf("конструктор: %v", err)
	}
	sendErr := s.Send(context.Background(), notifychannel.Target{Email: "a@example.com"},
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
