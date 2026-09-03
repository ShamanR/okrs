package callback

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"okrs/internal/platform/logging"
)

func loginRecord(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 1 || lines[0] == "" {
		t.Fatalf("ожидалась ровно одна запись, получено %d: %s", len(lines), buf.String())
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("строка не является валидным JSON: %v\n%s", err, lines[0])
	}
	return rec
}

func loggingHandler(buf *bytes.Buffer) *Handler {
	return New(nil, logging.New(logging.Config{Output: buf}), &fakeOnboarder{}, &fakeSessions{})
}

// Успешный вход обязан оставлять след: без него в логе не видно, кто и когда
// получил доступ, и расследование инцидента начинать не с чего.
func TestSuccessfulLoginIsLogged(t *testing.T) {
	buf := &bytes.Buffer{}
	ctx := logging.WithRequestID(context.Background(), "req-login")

	loggingHandler(buf).logLogin(ctx, 42, "keycloak")

	rec := loginRecord(t, buf)
	want := map[string]any{
		logging.KeyEvent:     logging.EventAuthLogin,
		"level":              "INFO",
		logging.KeyActorID:   float64(42),
		"provider":           "keycloak",
		logging.KeyRequestID: "req-login",
	}
	for k, v := range want {
		if rec[k] != v {
			t.Errorf("%s = %v, ожидалось %v", k, rec[k], v)
		}
	}
}

// Вход не должен раскрывать личность в открытом виде: адрес почты и
// отображаемое имя в логах запрещены, пользователь опознаётся идентификатором.
//
// Проверяется весь набор ключей, а не отсутствие конкретных значений: значений,
// которых нет во входных данных метода, в выводе не окажется и по случайности,
// поэтому такая проверка не поймала бы добавленное поле. Набор ключей — поймает.
func TestLoginRecordCarriesNoPersonalData(t *testing.T) {
	buf := &bytes.Buffer{}

	loggingHandler(buf).logLogin(context.Background(), 42, "keycloak")

	allowed := map[string]bool{
		"time": true, "level": true, "msg": true,
		logging.KeyEvent: true, logging.KeyService: true, logging.KeyEnv: true,
		logging.KeyActorID: true, "provider": true,
	}
	var unexpected []string
	for k := range loginRecord(t, buf) {
		if !allowed[k] {
			unexpected = append(unexpected, k)
		}
	}
	if len(unexpected) > 0 {
		sort.Strings(unexpected)
		t.Errorf("в записи о входе появились поля %v: любое из них может нести имя или адрес почты", unexpected)
	}
}
