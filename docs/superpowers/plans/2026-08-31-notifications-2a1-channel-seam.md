# Уведомления, фаза 2a-1: канал как сущность — план реализации

> **Для агентов:** ОБЯЗАТЕЛЬНЫЙ SUB-SKILL: используй superpowers:subagent-driven-development (рекомендуется) или superpowers:executing-plans, чтобы выполнять план задача за задачей. Шаги размечены чекбоксами (`- [ ]`).

**Цель:** канал доставки становится настраиваемой сущностью — публичный контракт, конфигурация в тенанте с шифрованными секретами, разрешение канала системным администратором, экран настройки в админке пространства и один рабочий канал (Mattermost) с кнопкой «Проверить».

**Архитектура:** канал — это публичный пакет-контракт `okrs/notifychannel` (только типы) плюс реализации, подключаемые опцией `app.Config.NotificationChannels` рядом с `main`. Глобального реестра нет: неподключённый канал не компилируется в бинарь. Ядро шифрует секрет по имени поля из `Descriptor` и отдаёт каналу расшифрованное значение — канал про хранение не знает. Список каналов, доступных пространству, задаёт системный администратор через `entitlement.notifications.<name>`.

**Стек:** Go 1.25, PostgreSQL + pgx/v5, AES-256-GCM из стандартной библиотеки, chi, React 18 без сборщика.

**Спека:** [`docs/superpowers/specs/2026-08-26-notifications-design.md`](../specs/2026-08-26-notifications-design.md) — разделы §7.4, §7.5, §10, §10.3, §10.5, §13.4, §13.5. План опирается на неё; исполнителю нужно прочитать обе.

**Предусловие:** фазы [1a](2026-08-27-notifications-1a-event-bus.md) и [1b](2026-08-27-notifications-1b-in-app.md) приняты. In-app уведомления работают.

**Место в фазе 2a.** Это первая половина. Вторая — [`2026-08-31-notifications-2a2-delivery.md`](2026-08-31-notifications-2a2-delivery.md): строки доставки, воркер отправки, колонки каналов в настройках пользователя. Разделены потому, что 2a-1 заканчивается проверяемым результатом — администратор пространства настраивает Mattermost и жмёт «Проверить», получая сообщение в мессенджер, — а автоматическая доставка по событиям требует отдельного цикла ревью.

**Чего в 2a-1 нет:** автоматической доставки уведомлений во внешние каналы (это 2a-2), Telegram (нужны одноразовые токены, deep-link и воркер приёма — отдельный срез), тарифов и интерфейса оплаты (§17 спеки).

## Глобальные ограничения

- **Коммиты не делает исполнитель** (правило 8 `CLAUDE.md`). Задачи заканчиваются прогоном тестов.
- **Схема БД меняется только миграцией** (правило 2 в `specs/010-architecture-constraints.md`).
- **Никакой бизнес-логики в handlers** (правило 1 там же).
- **Слои:** `handler → usecase → service → store`. Usecase не обращается к репозиториям; сервис работает с одним репозиторием; порт объявляется на стороне потребителя.
- **Именование:** store — множественное число, service — единственное. Алиас импорта `<entity>svc`.
- **Пакет на URI** для обработчиков.
- **Никаких N+1** (правило 9 `CLAUDE.md`); батчевые методы помечаются комментарием.
- **CSRF** обязателен для всех POST/PUT/PATCH/DELETE из браузера (правило 7 в `specs/010`).
- **Плейнтекст секрета никогда не покидает сервер** — наружу только маска `secret_hint`.
- **`gofmt -l internal/` непригоден как гейт** — дерево преимущественно CRLF, выдаёт ~450 пре-существующих файлов. Проверять точечно.
- **`go test ./...` сейчас полностью зелёный** и обязан таким остаться.
- **Язык:** комментарии в production-коде Go — английский; в `_test.go` — по фактической конвенции репозитория (русские пояснения в 120 из 217 тестовых файлов, см. Ruling 4). В JS — русский. Пользовательские строки и спеки — русские.

---

## Карта файлов

**Создаются:**

| Файл | Ответственность |
|---|---|
| `notifychannel/notifychannel.go` | **публичный** контракт: `Target`, `Message`, `Sender`, `Settings`, `Field`, `Descriptor`, `Channel`, `Linker` |
| `notifychannel/mattermost/mattermost.go` | **публичная** реализация канала Mattermost |
| `notifychannel/mattermost/mattermost_test.go` | тесты против `httptest`-сервера |
| `internal/platform/secretbox/secretbox.go` | AES-256-GCM: `Seal`/`Open`, ключ из env, маска секрета |
| `internal/platform/secretbox/secretbox_test.go` | round-trip, порча шифротекста, отсутствие ключа |
| `migrations/045_notification_channels.up.sql` / `.down.sql` | `notification_channels`, `notification_identities` |
| `internal/store/notificationchannels/notificationchannels.go` | конфигурация каналов тенанта + привязки аккаунтов |
| `internal/store/notificationchannels/notificationchannels_test.go` | тесты репозитория |
| `internal/service/notificationchannel/notificationchannel.go` | конфиг канала, шифрование, резолв `Sender`, гейт по entitlements |
| `internal/http/handlers/api/v1/system/notificationchannels/` | `GET /api/v1/system/notification-channels` |
| `internal/http/handlers/api/v1/admin/settings/notifications/` | `GET`/`PUT` конфигурации каналов тенанта + `POST .../test` |
| `internal/http/dto/notificationchannel.go` | DTO дескрипторов и конфигурации |

**Изменяются:**

| Файл | Что |
|---|---|
| `app/app.go` | опция `NotificationChannels`, карта каналов, проверка уникальности имён |
| `cmd/server/main.go` | подключение коробочного Mattermost |
| `internal/http/server.go` | `Options.NotificationChannels`, регистрация трёх маршрутов |
| `internal/http/httpdeps/httpdeps.go` | сервис каналов в графе |
| `internal/store/store.go` | новый репозиторий в composite |
| `web/static/system.js` | блок «Каналы уведомлений» в разделе Entitlements |
| `web/static/admin.js` | секция «Уведомления» в настройках пространства |
| `web/static/admin.css` | стили карточек каналов |
| `internal/http/testdata/routes.golden`, `specs/070-code-structure.md` | три новых маршрута |
| `specs/{010,040,050}.md`, `README.md` | правки спек |

---

## Task 1: Публичный контракт канала

Первый публичный пакет модуля после `app` и `web`. Он существует ровно затем, чтобы автор канала из **чужого репозитория** мог написать реализацию, не имея доступа к `internal/`.

**Файлы:**
- Создать: `notifychannel/notifychannel.go`
- Тест: `notifychannel/notifychannel_test.go`

**Интерфейсы:**
- Производит: `notifychannel.{Target, Message, Sender, Settings, FieldKind, Field, Descriptor, Channel, Linker}` и константы `FieldText`, `FieldURL`, `FieldSecret`. Используют задачи 4, 5, 6, 7.

- [ ] **Шаг 1: Написать падающий тест**

`notifychannel/notifychannel_test.go`:

```go
package notifychannel_test

import (
	"context"
	"testing"

	"okrs/notifychannel"
)

// Пакет обязан оставаться контрактом без зависимостей: автор канала из чужого
// модуля получает только типы. Тест фиксирует, что Channel собирается из
// Descriptor и конструктора, и что Sender удовлетворяется обычной функцией.
type fakeSender struct{ sent []notifychannel.Message }

func (f *fakeSender) Send(_ context.Context, _ notifychannel.Target, m notifychannel.Message) error {
	f.sent = append(f.sent, m)
	return nil
}

func TestChannelComposesDescriptorAndConstructor(t *testing.T) {
	ch := notifychannel.Channel{
		Descriptor: notifychannel.Descriptor{
			Name:        "fake",
			Title:       "Тестовый канал",
			SecretField: "token",
			Fields: []notifychannel.Field{
				{Key: "base_url", Label: "URL", Required: true, Kind: notifychannel.FieldURL},
				{Key: "token", Label: "Токен", Required: true, Kind: notifychannel.FieldSecret},
			},
		},
		New: func(s notifychannel.Settings) (notifychannel.Sender, error) {
			if s.Secret == "" {
				return nil, notifychannel.ErrMissingSecret
			}
			return &fakeSender{}, nil
		},
	}

	if _, err := ch.New(notifychannel.Settings{}); err == nil {
		t.Fatal("конструктор обязан отвергать пустой секрет")
	}
	sender, err := ch.New(notifychannel.Settings{Secret: "s", Values: map[string]any{"base_url": "https://x"}})
	if err != nil {
		t.Fatalf("конструктор: %v", err)
	}
	if err := sender.Send(context.Background(), notifychannel.Target{Email: "a@b.c"},
		notifychannel.Message{Title: "t", Body: "b"}); err != nil {
		t.Fatalf("send: %v", err)
	}
}

// SecretField называет поле, которое ядро шифрует. Дескриптор без секрета
// («SecretField: \"\"») — законный случай: канал может не требовать секрета.
func TestDescriptorMayHaveNoSecret(t *testing.T) {
	d := notifychannel.Descriptor{Name: "open", Title: "Без секрета"}
	if d.SecretField != "" {
		t.Fatal("пустой SecretField — валидное состояние")
	}
}

// Linker необязателен: канал, резолвящий адресата по email, его не реализует.
// Тест фиксирует, что интерфейс проверяется приведением типа, а не полем.
func TestLinkerIsOptional(t *testing.T) {
	var s notifychannel.Sender = &fakeSender{}
	if _, ok := s.(notifychannel.Linker); ok {
		t.Fatal("канал без LinkURL не должен удовлетворять Linker")
	}
}
```

- [ ] **Шаг 2: Прогнать тест и убедиться, что он падает**

Запустить: `go test ./notifychannel/ -v`
Ожидается: FAIL — пакета нет.

- [ ] **Шаг 3: Написать контракт**

`notifychannel/notifychannel.go`:

```go
// Package notifychannel is the public contract of a notification delivery channel.
//
// It holds types only: no I/O, no imports of okrs/internal/**. That is deliberate.
// A channel author in a separate module cannot import okrs/internal/... — Go's
// visibility rule forbids it — so a channel seam built on an internal registry
// would be unusable from outside. Channels are supplied instead through
// app.Config.NotificationChannels, assembled next to main.
package notifychannel

import (
	"context"
	"errors"
)

// ErrMissingSecret is what a channel constructor returns when its Descriptor
// declares a SecretField but Settings.Secret is empty — typically because the
// deployment has no encryption key configured.
var ErrMissingSecret = errors.New("notifychannel: secret is required but empty")

// Target is the addressee. A channel uses whichever field it can address by.
type Target struct {
	// ExternalID is a stored account link (a Telegram chat id, a Mattermost user id).
	ExternalID string
	// Email lets a channel resolve the addressee itself, with no linking step.
	Email string
}

// Message is what the core hands a channel. Title and Body are already rendered
// by internal/render/notify, so every channel says the same thing.
type Message struct {
	Title string
	Body  string
	// URL is an absolute or site-relative link back to the goal, may be empty.
	URL string
}

// Sender delivers one message. Implementations must be safe for concurrent use:
// the delivery worker runs several at once.
type Sender interface {
	Send(ctx context.Context, target Target, msg Message) error
}

// Settings is a channel's configuration inside one tenant. Secret arrives already
// decrypted; storage and encryption are the core's business, not the channel's.
type Settings struct {
	Values map[string]any
	Secret string
}

// FieldKind tells the admin UI how to render a configuration field.
type FieldKind string

const (
	FieldText   FieldKind = "text"
	FieldURL    FieldKind = "url"
	FieldSecret FieldKind = "secret"
)

// Field is one input in the channel's configuration form.
type Field struct {
	Key      string
	Label    string
	Hint     string
	Required bool
	Kind     FieldKind
}

// Descriptor describes the channel to the core and to the admin UI. Because the
// UI renders from this, the admin screen knows nothing about any specific channel
// and a channel from another repository appears in it unchanged.
type Descriptor struct {
	// Name is the channel key: stored in the database, echoed in user preferences,
	// and used to build the entitlement key entitlement.notifications.<Name>.
	Name  string
	Title string
	// SecretField names the field the core encrypts at rest. Empty means the
	// channel has no secret.
	SecretField string
	Fields      []Field
}

// Channel is one unit of wiring: how to describe it, and how to build a Sender
// from a tenant's settings.
type Channel struct {
	Descriptor Descriptor
	New        func(Settings) (Sender, error)
}

// Linker is implemented by a channel that needs an explicit account link — a
// one-time token and a deep link — rather than resolving the addressee by email.
// Optional: a channel without it is addressed through Target.Email.
type Linker interface {
	LinkURL(s Settings, token string) string
}
```

- [ ] **Шаг 4: Прогнать тесты**

Запустить: `go test ./notifychannel/ -count=1 -v && gofmt -l notifychannel/`
Ожидается: PASS, три теста; `gofmt` молчит.

- [ ] **Шаг 5: Доказать, что пакет остался чистым контрактом**

Это его единственное назначение: если он потянет `internal/`, автор канала из чужого модуля не сможет его импортировать, и весь сейм теряет смысл.

Запустить: `go list -deps ./notifychannel/ | rg 'okrs/'`
Ожидается: вывод содержит **только** `okrs/notifychannel`. Ни одного `okrs/internal/...`.

Запустить: `go list -deps ./notifychannel/ | rg -c 'database/sql|net/http|pgx'`
Ожидается: `0` — ни драйвера БД, ни HTTP-клиента в зависимостях контракта.

---

## Task 2: Шифрование секретов

Отдельный пакет, а не функции внутри сервиса: шифрование секрета — инфраструктурная забота, у неё свои тесты, и в фазе 2b её захочет переиспользовать канал с OAuth-токеном.

**Файлы:**
- Создать: `internal/platform/secretbox/secretbox.go`
- Тест: `internal/platform/secretbox/secretbox_test.go`

**Интерфейсы:**
- Производит:
  - `secretbox.New(keyB64 string) (*Box, error)` — ключ из env, 32 байта в base64
  - `(*Box).Seal(plaintext string) ([]byte, error)`
  - `(*Box).Open(ciphertext []byte) (string, error)`
  - `secretbox.Hint(plaintext string) string` — маска вида `••••4821`
  - `secretbox.ErrNoKey` — ключ не задан
- Используют: задачи 4 (сервис каналов), 6 (сборка).

- [ ] **Шаг 1: Написать падающий тест**

`internal/platform/secretbox/secretbox_test.go`:

```go
package secretbox_test

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"okrs/internal/platform/secretbox"
)

func newKey(t *testing.T) string {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return base64.StdEncoding.EncodeToString(k)
}

func TestSealOpenRoundTrip(t *testing.T) {
	b, err := secretbox.New(newKey(t))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	const secret = "xoxb-очень-секретный-токен-4821"
	ct, err := b.Seal(secret)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if strings.Contains(string(ct), "секретный") {
		t.Fatal("плейнтекст виден в шифротексте")
	}
	got, err := b.Open(ct)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if got != secret {
		t.Fatalf("round-trip: got %q", got)
	}
}

// Каждое шифрование обязано давать новый nonce, иначе одинаковые секреты
// разных тенантов дадут одинаковый шифротекст и станут сравнимы по БД.
func TestSealIsNonDeterministic(t *testing.T) {
	b, _ := secretbox.New(newKey(t))
	a, _ := b.Seal("одно и то же")
	c, _ := b.Seal("одно и то же")
	if string(a) == string(c) {
		t.Fatal("два шифрования одного значения совпали — nonce не случаен")
	}
}

// Испорченный шифротекст обязан давать ошибку, а не тихо возвращать мусор:
// GCM аутентифицирует данные, и эта проверка фиксирует, что мы её не потеряли.
func TestOpenRejectsTamperedCiphertext(t *testing.T) {
	b, _ := secretbox.New(newKey(t))
	ct, _ := b.Seal("секрет")
	ct[len(ct)-1] ^= 0xFF
	if _, err := b.Open(ct); err == nil {
		t.Fatal("порча шифротекста должна давать ошибку")
	}
}

// Чужой ключ не должен расшифровывать: иначе ротация ключа была бы бессмысленна.
func TestOpenWithWrongKeyFails(t *testing.T) {
	b1, _ := secretbox.New(newKey(t))
	b2, _ := secretbox.New(newKey(t))
	ct, _ := b1.Seal("секрет")
	if _, err := b2.Open(ct); err == nil {
		t.Fatal("другой ключ не должен расшифровывать")
	}
}

// Отсутствие ключа — не паника и не молчаливая работа без шифрования, а явная
// ошибка: коробочная установка без ключа просто не получает каналов с секретом.
func TestNewWithoutKeyReturnsErrNoKey(t *testing.T) {
	if _, err := secretbox.New(""); !errors.Is(err, secretbox.ErrNoKey) {
		t.Fatalf("got %v, want ErrNoKey", err)
	}
}

func TestNewRejectsWrongKeyLength(t *testing.T) {
	short := base64.StdEncoding.EncodeToString([]byte("слишком короткий"))
	if _, err := secretbox.New(short); err == nil || errors.Is(err, secretbox.ErrNoKey) {
		t.Fatalf("короткий ключ должен давать ошибку длины, got %v", err)
	}
}

// Маска показывается в UI вместо секрета. Она обязана быть узнаваемой, но не
// восстановимой: четыре последних символа — компромисс, принятый в спеке §7.4.
func TestHintShowsOnlyTail(t *testing.T) {
	if got := secretbox.Hint("xoxb-abcdef4821"); got != "••••4821" {
		t.Fatalf("got %q, want ••••4821", got)
	}
	// Короткий секрет не должен раскрываться целиком.
	if got := secretbox.Hint("abc"); strings.Contains(got, "abc") {
		t.Fatalf("короткий секрет раскрыт: %q", got)
	}
	if got := secretbox.Hint(""); got != "" {
		t.Fatalf("пустой секрет должен давать пустую маску, got %q", got)
	}
}
```

- [ ] **Шаг 2: Прогнать тест и убедиться, что он падает**

Запустить: `go test ./internal/platform/secretbox/ -v`
Ожидается: FAIL — пакета нет.

- [ ] **Шаг 3: Реализовать**

`internal/platform/secretbox/secretbox.go`:

```go
// Package secretbox encrypts channel secrets at rest with AES-256-GCM.
//
// It is a platform seam rather than a helper inside the channel service because
// encryption has its own failure modes worth testing separately, and because a
// second consumer (an OAuth token in a later phase) is already foreseeable.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// ErrNoKey means no encryption key is configured. Callers translate this into
// "channels with secrets are unavailable in this deployment" rather than failing
// the whole application: a box install owes no configuration for in-app alone.
var ErrNoKey = errors.New("secretbox: no encryption key configured")

// keyLen is 32 bytes — AES-256.
const keyLen = 32

type Box struct{ aead cipher.AEAD }

// New builds a Box from a base64-encoded 32-byte key, normally read from
// NOTIFICATIONS_SECRET_KEY.
func New(keyB64 string) (*Box, error) {
	if keyB64 == "" {
		return nil, ErrNoKey
	}
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("secretbox: key is not valid base64: %w", err)
	}
	if len(key) != keyLen {
		return nil, fmt.Errorf("secretbox: key must be %d bytes, got %d", keyLen, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secretbox: %w", err)
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext. The nonce is random per call and stored as the
// ciphertext's prefix, so two tenants holding the same secret produce different
// bytes and cannot be correlated by comparing rows.
func (b *Box) Seal(plaintext string) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("secretbox: nonce: %w", err)
	}
	return b.aead.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// Open decrypts. GCM authenticates, so a tampered or truncated value returns an
// error rather than plausible garbage.
func (b *Box) Open(ciphertext []byte) (string, error) {
	ns := b.aead.NonceSize()
	if len(ciphertext) < ns {
		return "", errors.New("secretbox: ciphertext shorter than nonce")
	}
	out, err := b.aead.Open(nil, ciphertext[:ns], ciphertext[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("secretbox: %w", err)
	}
	return string(out), nil
}

// Hint renders the mask shown in the admin UI in place of a secret: the last four
// characters, prefixed by dots. A secret too short to mask is hidden entirely —
// showing "••ab" of a four-character token would give away most of it.
func Hint(plaintext string) string {
	const tail = 4
	if plaintext == "" {
		return ""
	}
	r := []rune(plaintext)
	if len(r) <= tail*2 {
		return "••••"
	}
	return "••••" + string(r[len(r)-tail:])
}
```

- [ ] **Шаг 4: Прогнать тесты**

Запустить: `go test ./internal/platform/secretbox/ -count=1 -v && gofmt -l internal/platform/secretbox/`
Ожидается: PASS, семь тестов; `gofmt` молчит.

- [ ] **Шаг 5: Проверить, что пакет не тянет слои выше**

Запустить: `go list -deps ./internal/platform/secretbox/ | rg 'okrs/internal/(store|service|usecase|http)'`
Ожидается: пустой вывод. Пакет зависит только от стандартной библиотеки.

---

## Task 3: Миграция и репозиторий каналов

**Файлы:**
- Создать: `migrations/045_notification_channels.up.sql`, `.down.sql`
- Создать: `internal/store/notificationchannels/notificationchannels.go`
- Тест: `internal/store/notificationchannels/notificationchannels_test.go`
- Изменить: `internal/store/store.go`

**Интерфейсы:**
- Производит:
  - `notificationchannels.Config{Channel string; Enabled bool; Values map[string]any; SecretEnc []byte; SecretHint string; UpdatedAt time.Time; UpdatedByUserID *int64}`
  - `(*Repository).List(ctx, scope) ([]Config, error)`
  - `(*Repository).Get(ctx, scope, channel string) (Config, bool, error)`
  - `(*Repository).Upsert(ctx, scope, c Config, byUserID int64) error`
  - `notificationchannels.Identity{UserID int64; Channel, ExternalID, ExternalUsername string}`
  - `(*Repository).GetIdentity(ctx, scope, userID int64, channel string) (Identity, bool, error)`
  - `(*Repository).UpsertIdentity(ctx, scope, id Identity) error`
- Используют: задачи 4 (сервис), и фаза 2a-2 (доставка).

- [ ] **Шаг 1: Написать миграцию**

`migrations/045_notification_channels.up.sql`. Таблица доставок (`notification_deliveries`, §7.3 спеки) здесь **не создаётся** — она нужна фазе 2a-2 и без воркера была бы пустой. Таблица одноразовых токенов (`notification_link_tokens`, §7.5) тоже: она обслуживает `Linker`, который нужен Telegram, а не Mattermost.

```sql
-- Конфигурация канала в конкретном тенанте. Секрет лежит зашифрованным
-- (AES-256-GCM, nonce внутри значения); наружу в API уходит только secret_hint.
-- Имя канала — свободный текст без CHECK: набор каналов задаётся сборкой
-- (app.Config.NotificationChannels), и канал из внешнего репозитория обязан
-- работать без миграции.
CREATE TABLE notification_channels (
    tenant_id          BIGINT  NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    channel            TEXT    NOT NULL,
    enabled            BOOLEAN NOT NULL DEFAULT FALSE,
    config_json        JSONB   NOT NULL DEFAULT '{}',
    secret_enc         BYTEA,
    secret_hint        TEXT,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    PRIMARY KEY (tenant_id, channel)
);

-- Привязка аккаунта пользователя к внешнему каналу. Для Mattermost заполняется
-- автоматически при первой отправке (кэш резолва по email); для каналов с
-- Linker — по одноразовому токену.
CREATE TABLE notification_identities (
    tenant_id         BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id           BIGINT NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
    channel           TEXT   NOT NULL,
    external_id       TEXT   NOT NULL,
    external_username TEXT,
    linked_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, channel),
    -- Один внешний аккаунт не может принадлежать двум пользователям тенанта:
    -- иначе уведомления одного человека уедут другому.
    UNIQUE (tenant_id, channel, external_id)
);
```

`migrations/045_notification_channels.down.sql`:

```sql
DROP TABLE IF EXISTS notification_identities;
DROP TABLE IF EXISTS notification_channels;
```

- [ ] **Шаг 2: Написать падающий тест репозитория**

`internal/store/notificationchannels/notificationchannels_test.go`:

```go
package notificationchannels_test

import (
	"context"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/store/notificationchannels"
	"okrs/internal/store/testutil"
)

func newRepo(t *testing.T) (*notificationchannels.Repository, context.Context, domain.TenantScope, func()) {
	t.Helper()
	pool, cleanup := testutil.SetupDB(t)
	return notificationchannels.NewRepository(pool), context.Background(), domain.TenantScope{TenantID: 1}, cleanup
}

func TestUpsertThenGet(t *testing.T) {
	repo, ctx, scope, cleanup := newRepo(t)
	defer cleanup()

	in := notificationchannels.Config{
		Channel: "mattermost", Enabled: true,
		Values:     map[string]any{"base_url": "https://mm.example.com"},
		SecretEnc:  []byte{0xDE, 0xAD, 0xBE, 0xEF},
		SecretHint: "••••4821",
	}
	if err := repo.Upsert(ctx, scope, in, 1); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, ok, err := repo.Get(ctx, scope, "mattermost")
	if err != nil || !ok {
		t.Fatalf("get: %v ok=%v", err, ok)
	}
	if !got.Enabled || got.Values["base_url"] != "https://mm.example.com" {
		t.Fatalf("конфигурация не сохранилась: %+v", got)
	}
	if string(got.SecretEnc) != string(in.SecretEnc) {
		t.Fatalf("секрет не сохранился побайтово: %v", got.SecretEnc)
	}
	if got.UpdatedByUserID == nil || *got.UpdatedByUserID != 1 {
		t.Fatalf("не записан автор правки: %+v", got.UpdatedByUserID)
	}
}

// Повторный Upsert обязан обновлять, а не дублировать: ключ (tenant, channel).
func TestUpsertIsIdempotentPerChannel(t *testing.T) {
	repo, ctx, scope, cleanup := newRepo(t)
	defer cleanup()

	c := notificationchannels.Config{Channel: "mattermost", Enabled: false, Values: map[string]any{}}
	if err := repo.Upsert(ctx, scope, c, 1); err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	c.Enabled = true
	if err := repo.Upsert(ctx, scope, c, 1); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}
	list, err := repo.List(ctx, scope)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ожидалась одна строка, got %d", len(list))
	}
	if !list[0].Enabled {
		t.Fatal("второй upsert не обновил enabled")
	}
}

// Конфигурация одного тенанта не видна в другом.
func TestTenantIsolation(t *testing.T) {
	repo, ctx, scope, cleanup := newRepo(t)
	defer cleanup()

	if err := repo.Upsert(ctx, scope, notificationchannels.Config{
		Channel: "mattermost", Values: map[string]any{},
	}, 1); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	list, err := repo.List(ctx, domain.TenantScope{TenantID: 999})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("чужой тенант видит %d каналов", len(list))
	}
}

// Отсутствующий канал — не ошибка: тенант мог его никогда не настраивать.
func TestGetMissingChannelIsNotAnError(t *testing.T) {
	repo, ctx, scope, cleanup := newRepo(t)
	defer cleanup()

	_, ok, err := repo.Get(ctx, scope, "nope")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if ok {
		t.Fatal("несуществующий канал не должен находиться")
	}
}

func TestIdentityRoundTrip(t *testing.T) {
	repo, ctx, scope, cleanup := newRepo(t)
	defer cleanup()

	id := notificationchannels.Identity{
		UserID: 1, Channel: "mattermost",
		ExternalID: "mm-user-77", ExternalUsername: "ivan",
	}
	if err := repo.UpsertIdentity(ctx, scope, id); err != nil {
		t.Fatalf("upsert identity: %v", err)
	}
	got, ok, err := repo.GetIdentity(ctx, scope, 1, "mattermost")
	if err != nil || !ok {
		t.Fatalf("get identity: %v ok=%v", err, ok)
	}
	if got.ExternalID != "mm-user-77" || got.ExternalUsername != "ivan" {
		t.Fatalf("got %+v", got)
	}
}

// Один внешний аккаунт не может принадлежать двум пользователям одного тенанта.
// Пользователь id=2 (system:migration) заводится миграциями, поэтому доступен.
func TestExternalIDIsUniquePerTenantChannel(t *testing.T) {
	repo, ctx, scope, cleanup := newRepo(t)
	defer cleanup()

	first := notificationchannels.Identity{UserID: 1, Channel: "mattermost", ExternalID: "shared"}
	if err := repo.UpsertIdentity(ctx, scope, first); err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	second := notificationchannels.Identity{UserID: 2, Channel: "mattermost", ExternalID: "shared"}
	if err := repo.UpsertIdentity(ctx, scope, second); err == nil {
		t.Fatal("тот же external_id у второго пользователя должен отвергаться")
	}
}
```

- [ ] **Шаг 3: Прогнать тест и убедиться, что он падает**

Запустить: `go test ./internal/store/notificationchannels/ -v`
Ожидается: FAIL — пакета нет. Тесты требуют Docker; скипнувшийся тест результатом не является.

- [ ] **Шаг 4: Реализовать репозиторий**

`internal/store/notificationchannels/notificationchannels.go`. Форму берём с соседа `internal/store/notifications/notifications.go`: конструктор, `domain.TenantScope` первым аргументом после ctx, JSONB через `encoding/json`.

```go
// Package notificationchannels persists per-tenant channel configuration and the
// account links a channel needs to address a user.
//
// The secret is stored already encrypted; this package never sees plaintext and
// never decrypts — that belongs to service/notificationchannel, which owns the key.
package notificationchannels

import (
	"context"
	"encoding/json"
	"time"

	"okrs/internal/core/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// Config is one channel's configuration inside a tenant.
type Config struct {
	Channel         string
	Enabled         bool
	Values          map[string]any
	SecretEnc       []byte
	SecretHint      string
	UpdatedAt       time.Time
	UpdatedByUserID *int64
}

// Identity links a user to their account in an external channel.
type Identity struct {
	UserID           int64
	Channel          string
	ExternalID       string
	ExternalUsername string
	LinkedAt         time.Time
}

const configCols = `channel, enabled, config_json, secret_enc, secret_hint, updated_at, updated_by_user_id`

func scanConfig(row pgx.Row) (Config, error) {
	var c Config
	var raw []byte
	err := row.Scan(&c.Channel, &c.Enabled, &raw, &c.SecretEnc, &c.SecretHint, &c.UpdatedAt, &c.UpdatedByUserID)
	if err != nil {
		return Config{}, err
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &c.Values)
	}
	if c.Values == nil {
		c.Values = map[string]any{}
	}
	return c, nil
}

// List returns every channel the tenant has ever configured, enabled or not.
func (r *Repository) List(ctx context.Context, scope domain.TenantScope) ([]Config, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+configCols+` FROM notification_channels WHERE tenant_id = $1 ORDER BY channel`,
		scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Config
	for rows.Next() {
		c, err := scanConfig(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Get returns one channel's configuration. A missing row is not an error: a tenant
// that never configured the channel is the normal case.
func (r *Repository) Get(ctx context.Context, scope domain.TenantScope, channel string) (Config, bool, error) {
	c, err := scanConfig(r.db.QueryRow(ctx,
		`SELECT `+configCols+` FROM notification_channels WHERE tenant_id = $1 AND channel = $2`,
		scope.TenantID, channel))
	if err == pgx.ErrNoRows {
		return Config{}, false, nil
	}
	if err != nil {
		return Config{}, false, err
	}
	return c, true, nil
}

// Upsert writes the configuration, recording who changed it.
func (r *Repository) Upsert(ctx context.Context, scope domain.TenantScope, c Config, byUserID int64) error {
	values := c.Values
	if values == nil {
		values = map[string]any{}
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO notification_channels
			(tenant_id, channel, enabled, config_json, secret_enc, secret_hint, updated_at, updated_by_user_id)
		VALUES ($1,$2,$3,$4,$5,$6, now(), $7)
		ON CONFLICT (tenant_id, channel) DO UPDATE
		   SET enabled            = EXCLUDED.enabled,
		       config_json        = EXCLUDED.config_json,
		       secret_enc         = EXCLUDED.secret_enc,
		       secret_hint        = EXCLUDED.secret_hint,
		       updated_at         = now(),
		       updated_by_user_id = EXCLUDED.updated_by_user_id`,
		scope.TenantID, c.Channel, c.Enabled, raw, c.SecretEnc, c.SecretHint, byUserID)
	return err
}

// GetIdentity returns a user's account link for one channel.
func (r *Repository) GetIdentity(ctx context.Context, scope domain.TenantScope, userID int64, channel string) (Identity, bool, error) {
	var id Identity
	var username *string
	err := r.db.QueryRow(ctx, `
		SELECT user_id, channel, external_id, external_username, linked_at
		  FROM notification_identities
		 WHERE tenant_id = $1 AND user_id = $2 AND channel = $3`,
		scope.TenantID, userID, channel,
	).Scan(&id.UserID, &id.Channel, &id.ExternalID, &username, &id.LinkedAt)
	if err == pgx.ErrNoRows {
		return Identity{}, false, nil
	}
	if err != nil {
		return Identity{}, false, err
	}
	if username != nil {
		id.ExternalUsername = *username
	}
	return id, true, nil
}

// UpsertIdentity stores or refreshes a link. The unique index on
// (tenant_id, channel, external_id) rejects the same external account being
// claimed by a second user — that would misdeliver one person's notifications.
func (r *Repository) UpsertIdentity(ctx context.Context, scope domain.TenantScope, id Identity) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO notification_identities
			(tenant_id, user_id, channel, external_id, external_username)
		VALUES ($1,$2,$3,$4,NULLIF($5,''))
		ON CONFLICT (tenant_id, user_id, channel) DO UPDATE
		   SET external_id       = EXCLUDED.external_id,
		       external_username = EXCLUDED.external_username,
		       linked_at         = now()`,
		scope.TenantID, id.UserID, id.Channel, id.ExternalID, id.ExternalUsername)
	return err
}
```

- [ ] **Шаг 5: Подключить в composite**

`internal/store/store.go` — поле и его инициализация, рядом с `NotificationPrefs`:

```go
	NotificationChannels *notificationchannels.Repository
```
```go
		NotificationChannels: notificationchannels.NewRepository(db),
```

- [ ] **Шаг 6: Прогнать тесты**

Запустить: `go build ./... && go test ./internal/store/notificationchannels/ ./internal/store/ -count=1 -v`
Ожидается: PASS, шесть тестов репозитория; тесты реально шли против контейнера.

- [ ] **Шаг 7: Проверить обратимость миграции**

Запустить цикл `up → down → up` через golang-migrate на живой БД, как это делалось для миграции 044.
Ожидается: версия 45 → 44 → 45, `dirty=false` на каждом шаге, ошибок нет.

---

## Task 4: Сервис каналов

Здесь сходятся четыре вещи: конфигурация из БД, шифрование, список каналов сборки и гейт по entitlements. Сервис — единственное место, где секрет существует в открытом виде.

**Файлы:**
- Создать: `internal/service/notificationchannel/notificationchannel.go`
- Тест: `internal/service/notificationchannel/notificationchannel_test.go`

**Интерфейсы:**
- Потребляет: `notificationchannels.Repository` (задача 3), `secretbox.Box` (задача 2), `notifychannel.Channel` (задача 1), `entitlements.Entitlements`.
- Производит:
  - `notificationchannel.New(repo Repo, box *secretbox.Box, channels []notifychannel.Channel, ent entitlements.Entitlements) (*Service, error)`
  - `(*Service).Descriptors() []notifychannel.Descriptor` — всё, что собрано в бинарь
  - `(*Service).Available(scope) []notifychannel.Descriptor` — только разрешённые тенанту
  - `(*Service).IsAvailable(scope, name string) bool`
  - `(*Service).List(ctx, scope) ([]ChannelState, error)` — разрешённые каналы с их конфигурацией
  - `(*Service).Save(ctx, scope, in SaveInput, byUserID int64) error`
  - `(*Service).Sender(ctx, scope, name string) (notifychannel.Sender, error)`
  - `(*Service).EnabledNames(ctx, scope) ([]string, error)` — для фазы 2a-2 и колонок в настройках
  - Ошибки: `ErrUnknownChannel`, `ErrNotAvailable`, `ErrNoSecretKey`, `ErrNotConfigured`
- Используют: задачи 6 (системная панель), 7 (админка), и вся фаза 2a-2.

**Ключевое соглашение об именах ключей entitlement — не перепутать, иначе гейт молча не сработает.** `provisioning.SetEntitlements` сам добавляет префикс (`internal/service/provisioning/provisioning.go:172`): системная панель пишет **голый** ключ `notifications.mattermost`, а в `tenant_settings` он лежит как `entitlement.notifications.mattermost`. Проверка `entitlements.Has(scope, key)` получает **полный** ключ. Отсюда:

```go
// entitlementKey builds the key the gate checks. Note the asymmetry with the
// system panel, which writes the bare form: provisioning.SetEntitlements adds the
// "entitlement." prefix, so what is written as "notifications.mattermost" is read
// back as "entitlement.notifications.mattermost".
func entitlementKey(name string) string { return "entitlement.notifications." + name }
```

- [ ] **Шаг 1: Написать падающий тест**

`internal/service/notificationchannel/notificationchannel_test.go`:

```go
package notificationchannel_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"

	"okrs/internal/core/domain"
	"okrs/internal/platform/secretbox"
	notificationchannelsvc "okrs/internal/service/notificationchannel"
	"okrs/internal/store/notificationchannels"
	"okrs/notifychannel"
)

// fakeRepo — стор в памяти: сервис не должен требовать БД для своей логики.
type fakeRepo struct{ rows map[string]notificationchannels.Config }

func (f *fakeRepo) List(context.Context, domain.TenantScope) ([]notificationchannels.Config, error) {
	out := make([]notificationchannels.Config, 0, len(f.rows))
	for _, c := range f.rows {
		out = append(out, c)
	}
	return out, nil
}

func (f *fakeRepo) Get(_ context.Context, _ domain.TenantScope, ch string) (notificationchannels.Config, bool, error) {
	c, ok := f.rows[ch]
	return c, ok, nil
}

func (f *fakeRepo) Upsert(_ context.Context, _ domain.TenantScope, c notificationchannels.Config, _ int64) error {
	if f.rows == nil {
		f.rows = map[string]notificationchannels.Config{}
	}
	f.rows[c.Channel] = c
	return nil
}

// gate — управляемая реализация entitlements: разрешает только перечисленные ключи.
type gate struct{ allow map[string]bool }

func (g gate) Has(_ domain.TenantScope, key string) bool { return g.allow[key] }
func (g gate) Limit(domain.TenantScope, string) int64    { return -1 }

type recordingSender struct{ built notifychannel.Settings }

func (r *recordingSender) Send(context.Context, notifychannel.Target, notifychannel.Message) error {
	return nil
}

func testChannel(built **recordingSender) notifychannel.Channel {
	return notifychannel.Channel{
		Descriptor: notifychannel.Descriptor{
			Name: "fake", Title: "Фейковый", SecretField: "token",
			Fields: []notifychannel.Field{
				{Key: "base_url", Label: "URL", Required: true, Kind: notifychannel.FieldURL},
				{Key: "token", Label: "Токен", Required: true, Kind: notifychannel.FieldSecret},
			},
		},
		New: func(s notifychannel.Settings) (notifychannel.Sender, error) {
			if s.Secret == "" {
				return nil, notifychannel.ErrMissingSecret
			}
			rs := &recordingSender{built: s}
			*built = rs
			return rs, nil
		},
	}
}

func newKey(t *testing.T) *secretbox.Box {
	t.Helper()
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	b, err := secretbox.New(base64.StdEncoding.EncodeToString(k))
	if err != nil {
		t.Fatalf("secretbox: %v", err)
	}
	return b
}

func newSvc(t *testing.T, allow map[string]bool, built **recordingSender) (*notificationchannelsvc.Service, *fakeRepo) {
	t.Helper()
	repo := &fakeRepo{rows: map[string]notificationchannels.Config{}}
	svc, err := notificationchannelsvc.New(repo, newKey(t),
		[]notifychannel.Channel{testChannel(built)}, gate{allow: allow})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	return svc, repo
}

var scope = domain.TenantScope{TenantID: 1}

// Канал, не разрешённый тенанту, не виден в списке доступных и не настраивается.
// Это и есть §10.5 спеки: список задаёт системный администратор, не тенант.
func TestUnentitledChannelIsInvisibleAndUnwritable(t *testing.T) {
	var built *recordingSender
	svc, repo := newSvc(t, map[string]bool{}, &built)

	if got := svc.Available(scope); len(got) != 0 {
		t.Fatalf("неразрешённый канал попал в доступные: %+v", got)
	}
	err := svc.Save(context.Background(), scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: true, Values: map[string]any{"base_url": "https://x"}, Secret: "s",
	}, 1)
	if !errors.Is(err, notificationchannelsvc.ErrNotAvailable) {
		t.Fatalf("got %v, want ErrNotAvailable", err)
	}
	if len(repo.rows) != 0 {
		t.Fatal("запись произошла, несмотря на отказ")
	}
}

// Разрешённый канал виден, а Descriptors() продолжает показывать ВСЁ, что собрано
// в бинарь: системной панели нужен полный список, чтобы было что разрешать.
func TestDescriptorsShowsBuildWhileAvailableShowsEntitled(t *testing.T) {
	var built *recordingSender
	svc, _ := newSvc(t, map[string]bool{}, &built)

	if len(svc.Descriptors()) != 1 {
		t.Fatal("Descriptors обязан показывать канал сборки независимо от entitlements")
	}
	if len(svc.Available(scope)) != 0 {
		t.Fatal("Available обязан фильтровать по entitlements")
	}
}

// Секрет уходит в БД зашифрованным и никогда не возвращается наружу открытым.
func TestSaveEncryptsSecretAndExposesOnlyHint(t *testing.T) {
	var built *recordingSender
	svc, repo := newSvc(t, map[string]bool{"entitlement.notifications.fake": true}, &built)

	const secret = "token-abcdef4821"
	if err := svc.Save(context.Background(), scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: true,
		Values: map[string]any{"base_url": "https://x"}, Secret: secret,
	}, 7); err != nil {
		t.Fatalf("save: %v", err)
	}
	row := repo.rows["fake"]
	if len(row.SecretEnc) == 0 {
		t.Fatal("секрет не зашифрован")
	}
	if string(row.SecretEnc) == secret {
		t.Fatal("секрет лежит в открытом виде")
	}
	if row.SecretHint != "••••4821" {
		t.Fatalf("маска: got %q", row.SecretHint)
	}

	states, err := svc.List(context.Background(), scope)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(states) != 1 || states[0].SecretHint != "••••4821" {
		t.Fatalf("состояние: %+v", states)
	}
	// В состоянии наружу не должно быть ни одного поля с плейнтекстом.
	if states[0].Values["token"] != nil {
		t.Fatal("секретное поле просочилось в Values")
	}
}

// Пустой секрет при сохранении означает «не менять», а не «стереть»: форма в
// админке показывает маску, и пользователь, правя только base_url, не должен
// потерять токен.
func TestEmptySecretKeepsPrevious(t *testing.T) {
	var built *recordingSender
	svc, repo := newSvc(t, map[string]bool{"entitlement.notifications.fake": true}, &built)
	ctx := context.Background()

	_ = svc.Save(ctx, scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: true, Values: map[string]any{"base_url": "https://a"}, Secret: "первый",
	}, 1)
	before := repo.rows["fake"].SecretEnc

	if err := svc.Save(ctx, scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: true, Values: map[string]any{"base_url": "https://b"}, Secret: "",
	}, 1); err != nil {
		t.Fatalf("save 2: %v", err)
	}
	after := repo.rows["fake"]
	if string(after.SecretEnc) != string(before) {
		t.Fatal("пустой секрет затёр сохранённый")
	}
	if after.Values["base_url"] != "https://b" {
		t.Fatal("несекретное поле не обновилось")
	}
}

// Sender собирается из расшифрованного секрета и несекретных значений.
func TestSenderReceivesDecryptedSettings(t *testing.T) {
	var built *recordingSender
	svc, _ := newSvc(t, map[string]bool{"entitlement.notifications.fake": true}, &built)
	ctx := context.Background()

	_ = svc.Save(ctx, scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: true,
		Values: map[string]any{"base_url": "https://mm"}, Secret: "секрет-77",
	}, 1)

	if _, err := svc.Sender(ctx, scope, "fake"); err != nil {
		t.Fatalf("sender: %v", err)
	}
	if built == nil {
		t.Fatal("конструктор канала не вызван")
	}
	if built.built.Secret != "секрет-77" {
		t.Fatalf("канал получил секрет %q", built.built.Secret)
	}
	if built.built.Values["base_url"] != "https://mm" {
		t.Fatalf("канал не получил значения: %+v", built.built.Values)
	}
}

// Ненастроенный канал даёт понятную ошибку, а не панику на пустом секрете.
func TestSenderForUnconfiguredChannel(t *testing.T) {
	var built *recordingSender
	svc, _ := newSvc(t, map[string]bool{"entitlement.notifications.fake": true}, &built)

	_, err := svc.Sender(context.Background(), scope, "fake")
	if !errors.Is(err, notificationchannelsvc.ErrNotConfigured) {
		t.Fatalf("got %v, want ErrNotConfigured", err)
	}
}

// Неизвестное имя канала отвергается до любой работы с БД.
func TestUnknownChannelRejected(t *testing.T) {
	var built *recordingSender
	svc, _ := newSvc(t, map[string]bool{"entitlement.notifications.fake": true}, &built)

	err := svc.Save(context.Background(), scope, notificationchannelsvc.SaveInput{Channel: "nope"}, 1)
	if !errors.Is(err, notificationchannelsvc.ErrUnknownChannel) {
		t.Fatalf("got %v, want ErrUnknownChannel", err)
	}
}

// Без ключа шифрования канал с секретом настроить нельзя, но само приложение
// обязано работать: коробочная установка без ключа живёт на одном in-app.
func TestWithoutSecretKeyChannelsWithSecretsAreUnavailable(t *testing.T) {
	var built *recordingSender
	repo := &fakeRepo{rows: map[string]notificationchannels.Config{}}
	svc, err := notificationchannelsvc.New(repo, nil,
		[]notifychannel.Channel{testChannel(&built)},
		gate{allow: map[string]bool{"entitlement.notifications.fake": true}})
	if err != nil {
		t.Fatalf("сервис обязан собираться без ключа: %v", err)
	}
	saveErr := svc.Save(context.Background(), scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: true, Secret: "s", Values: map[string]any{},
	}, 1)
	if !errors.Is(saveErr, notificationchannelsvc.ErrNoSecretKey) {
		t.Fatalf("got %v, want ErrNoSecretKey", saveErr)
	}
}

// Дубликат имени канала в сборке — ошибка сборки, а не молчаливое затирание.
func TestDuplicateChannelNameIsRejected(t *testing.T) {
	var built *recordingSender
	_, err := notificationchannelsvc.New(&fakeRepo{},
		newKey(t),
		[]notifychannel.Channel{testChannel(&built), testChannel(&built)},
		gate{})
	if err == nil {
		t.Fatal("два канала с одинаковым Descriptor.Name должны отвергаться")
	}
}
```

- [ ] **Шаг 2: Прогнать тест и убедиться, что он падает**

Запустить: `go test ./internal/service/notificationchannel/ -v`
Ожидается: FAIL — пакета нет. БД этому пакету не нужна: логика проверяется на фейках.

- [ ] **Шаг 3: Реализовать сервис**

`internal/service/notificationchannel/notificationchannel.go`:

```go
// Package notificationchannel owns everything about a channel's configuration in a
// tenant: which channels this build has, which of them the tenant is entitled to,
// what is stored for each, and how to turn that into a ready Sender.
//
// It is the only place where a channel secret exists in plaintext. The store keeps
// it encrypted, the API returns only a mask, and the channel receives it already
// decrypted — so no other layer has to be trusted with it.
package notificationchannel

import (
	"context"
	"errors"
	"fmt"

	"okrs/internal/core/domain"
	"okrs/internal/platform/entitlements"
	"okrs/internal/platform/secretbox"
	"okrs/internal/store/notificationchannels"
	"okrs/notifychannel"
)

var (
	// ErrUnknownChannel: the name is not in this build at all.
	ErrUnknownChannel = errors.New("notificationchannel: unknown channel")
	// ErrNotAvailable: the channel exists but this tenant has not been granted it.
	ErrNotAvailable = errors.New("notificationchannel: channel not available for this tenant")
	// ErrNoSecretKey: the deployment has no encryption key, so a channel that
	// requires a secret cannot be configured.
	ErrNoSecretKey = errors.New("notificationchannel: no encryption key configured")
	// ErrNotConfigured: the tenant has never saved this channel's settings.
	ErrNotConfigured = errors.New("notificationchannel: channel not configured")
)

// Repo is the port this service needs, declared consumer-side per specs/010.
type Repo interface {
	List(ctx context.Context, scope domain.TenantScope) ([]notificationchannels.Config, error)
	Get(ctx context.Context, scope domain.TenantScope, channel string) (notificationchannels.Config, bool, error)
	Upsert(ctx context.Context, scope domain.TenantScope, c notificationchannels.Config, byUserID int64) error
}

// ChannelState is one channel as the admin UI sees it: what it is, whether the
// tenant switched it on, and what is stored — with the secret reduced to a mask.
type ChannelState struct {
	Descriptor notifychannel.Descriptor
	Enabled    bool
	Configured bool
	Values     map[string]any
	SecretHint string
}

// SaveInput is one channel's configuration as submitted by a tenant admin.
// An empty Secret means "leave the stored one alone" — the form shows a mask,
// so a user editing only a URL must not lose the token.
type SaveInput struct {
	Channel string
	Enabled bool
	Values  map[string]any
	Secret  string
}

type Service struct {
	repo     Repo
	box      *secretbox.Box // nil when the deployment configured no key
	channels map[string]notifychannel.Channel
	order    []string // build order, so the UI lists channels deterministically
	ent      entitlements.Entitlements
}

// New assembles the service. A duplicate Descriptor.Name is an assembly error
// rather than a silent overwrite: two channels answering to one name would make
// which one you configured depend on map iteration order.
func New(repo Repo, box *secretbox.Box, channels []notifychannel.Channel, ent entitlements.Entitlements) (*Service, error) {
	s := &Service{repo: repo, box: box, channels: map[string]notifychannel.Channel{}, ent: ent}
	for _, ch := range channels {
		name := ch.Descriptor.Name
		if name == "" {
			return nil, errors.New("notificationchannel: channel with empty Descriptor.Name")
		}
		if _, dup := s.channels[name]; dup {
			return nil, fmt.Errorf("notificationchannel: duplicate channel name %q", name)
		}
		s.channels[name] = ch
		s.order = append(s.order, name)
	}
	return s, nil
}

// entitlementKey builds the key the gate checks. Note the asymmetry with the
// system panel, which writes the bare form: provisioning.SetEntitlements adds the
// "entitlement." prefix, so what is written as "notifications.mattermost" is read
// back as "entitlement.notifications.mattermost".
func entitlementKey(name string) string { return "entitlement.notifications." + name }

// Descriptors returns every channel compiled into this build, entitled or not.
// The system panel needs the full list — that is what it grants from.
func (s *Service) Descriptors() []notifychannel.Descriptor {
	out := make([]notifychannel.Descriptor, 0, len(s.order))
	for _, name := range s.order {
		out = append(out, s.channels[name].Descriptor)
	}
	return out
}

// IsAvailable reports whether the tenant may use this channel at all.
func (s *Service) IsAvailable(scope domain.TenantScope, name string) bool {
	if _, ok := s.channels[name]; !ok {
		return false
	}
	return s.ent.Has(scope, entitlementKey(name))
}

// Available returns only the channels this tenant was granted. The tenant admin
// screen renders from this, so a channel the tenant does not have never appears —
// no locked cards, no upsell (design spec §13.4).
func (s *Service) Available(scope domain.TenantScope) []notifychannel.Descriptor {
	out := make([]notifychannel.Descriptor, 0, len(s.order))
	for _, name := range s.order {
		if s.ent.Has(scope, entitlementKey(name)) {
			out = append(out, s.channels[name].Descriptor)
		}
	}
	return out
}

// List returns the available channels together with what the tenant stored.
func (s *Service) List(ctx context.Context, scope domain.TenantScope) ([]ChannelState, error) {
	rows, err := s.repo.List(ctx, scope)
	if err != nil {
		return nil, err
	}
	stored := make(map[string]notificationchannels.Config, len(rows))
	for _, r := range rows {
		stored[r.Channel] = r
	}

	var out []ChannelState
	for _, d := range s.Available(scope) {
		st := ChannelState{Descriptor: d, Values: map[string]any{}}
		if row, ok := stored[d.Name]; ok {
			st.Configured = true
			st.Enabled = row.Enabled
			st.SecretHint = row.SecretHint
			st.Values = sanitize(row.Values, d)
		}
		out = append(out, st)
	}
	return out, nil
}

// sanitize drops the secret field from the values that leave the server. The
// secret is stored encrypted in its own column, but a channel could also have
// written it into config_json by mistake; stripping it here means one bug cannot
// become a leak.
func sanitize(values map[string]any, d notifychannel.Descriptor) map[string]any {
	out := make(map[string]any, len(values))
	for k, v := range values {
		if d.SecretField != "" && k == d.SecretField {
			continue
		}
		out[k] = v
	}
	return out
}

// EnabledNames returns the channels the tenant has both been granted and switched
// on. Phase 2a-2's fan-out and the user settings screen read this.
func (s *Service) EnabledNames(ctx context.Context, scope domain.TenantScope) ([]string, error) {
	states, err := s.List(ctx, scope)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, st := range states {
		if st.Enabled {
			out = append(out, st.Descriptor.Name)
		}
	}
	return out, nil
}

// Save validates and stores one channel's configuration.
func (s *Service) Save(ctx context.Context, scope domain.TenantScope, in SaveInput, byUserID int64) error {
	ch, ok := s.channels[in.Channel]
	if !ok {
		return ErrUnknownChannel
	}
	if !s.ent.Has(scope, entitlementKey(in.Channel)) {
		return ErrNotAvailable
	}

	prev, hadPrev, err := s.repo.Get(ctx, scope, in.Channel)
	if err != nil {
		return err
	}

	row := notificationchannels.Config{
		Channel: in.Channel,
		Enabled: in.Enabled,
		Values:  sanitize(in.Values, ch.Descriptor),
	}

	switch {
	case ch.Descriptor.SecretField == "":
		// Channel has no secret; nothing to encrypt.
	case in.Secret != "":
		if s.box == nil {
			return ErrNoSecretKey
		}
		enc, err := s.box.Seal(in.Secret)
		if err != nil {
			return err
		}
		row.SecretEnc = enc
		row.SecretHint = secretbox.Hint(in.Secret)
	case hadPrev:
		// Empty secret with a stored one: keep it. The form shows a mask, so a
		// user editing only a URL must not silently lose the token.
		row.SecretEnc = prev.SecretEnc
		row.SecretHint = prev.SecretHint
	default:
		// Enabling a secret-requiring channel with no secret at all is refused by
		// the channel's own constructor at Sender() time; storing it disabled is
		// legitimate, so nothing to do here.
	}

	return s.repo.Upsert(ctx, scope, row, byUserID)
}

// Sender builds a ready-to-use Sender from the tenant's stored configuration.
func (s *Service) Sender(ctx context.Context, scope domain.TenantScope, name string) (notifychannel.Sender, error) {
	ch, ok := s.channels[name]
	if !ok {
		return nil, ErrUnknownChannel
	}
	if !s.ent.Has(scope, entitlementKey(name)) {
		return nil, ErrNotAvailable
	}
	row, ok, err := s.repo.Get(ctx, scope, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotConfigured
	}

	settings := notifychannel.Settings{Values: row.Values}
	if ch.Descriptor.SecretField != "" && len(row.SecretEnc) > 0 {
		if s.box == nil {
			return nil, ErrNoSecretKey
		}
		secret, err := s.box.Open(row.SecretEnc)
		if err != nil {
			return nil, fmt.Errorf("notificationchannel: decrypt %s: %w", name, err)
		}
		settings.Secret = secret
	}
	return ch.New(settings)
}
```

- [ ] **Шаг 4: Прогнать тесты**

Запустить: `go test ./internal/service/notificationchannel/ -count=1 -v && gofmt -l internal/service/notificationchannel/`
Ожидается: PASS, девять тестов; `gofmt` молчит.

- [ ] **Шаг 5: Доказать, что секрет не вытекает наружу**

Запустить: `rg -n 'SecretEnc|Secret\b' internal/service/notificationchannel/notificationchannel.go | rg -v '^\s*//'`
Ожидается глазами: `ChannelState` не содержит поля с плейнтекстом секрета — только `SecretHint`; единственные места, где секрет открыт, это аргумент `SaveInput.Secret` и локальная переменная в `Sender`.

Запустить: `go test ./internal/service/notificationchannel/ -run TestSaveEncrypts -count=1 -v`
Ожидается: PASS — тест проверяет и шифрование, и то, что секретное поле не просачивается в `Values`.

---

## Task 5: Канал Mattermost

Первая реализация контракта — и одновременно образец для автора канала из чужого репозитория. Живёт в **публичном** подпакете, потому что подключается опцией из `main`, а не blank-import'ом.

**Файлы:**
- Создать: `notifychannel/mattermost/mattermost.go`
- Тест: `notifychannel/mattermost/mattermost_test.go`

**Интерфейсы:**
- Потребляет: `notifychannel` (задача 1).
- Производит: `mattermost.Channel() notifychannel.Channel`.
- Используют: задачи 6 (подключение в `main`) и 7 (кнопка «Проверить»).

**Почему Mattermost первым.** Он адресуется по email и не требует ни привязки аккаунта, ни одноразовых токенов, ни фонового воркера приёма — то есть не реализует `Linker`. Telegram всё это требует и поэтому идёт отдельным срезом.

**Путь отправки — три вызова Mattermost API**, и все три нужны: бот не может написать в личку, не создав прямой канал, а для создания канала нужны оба идентификатора.

1. `GET /api/v4/users/me` — узнать собственный id бота (кэшируется в экземпляре `Sender`);
2. `GET /api/v4/users/email/{email}` — резолв адресата;
3. `POST /api/v4/channels/direct` с телом `[botID, userID]` — получить (или переиспользовать) прямой канал;
4. `POST /api/v4/posts` с `channel_id` и `message`.

- [ ] **Шаг 1: Написать падающий тест**

Тесты идут против `httptest.Server`, а не против живого Mattermost: нам важно, что канал делает верные запросы и верно разбирает ответы.

`notifychannel/mattermost/mattermost_test.go`:

```go
package mattermost_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"okrs/notifychannel"
	"okrs/notifychannel/mattermost"
)

// fakeMM изображает Mattermost: запоминает путь каждого запроса и отданный пост.
type fakeMM struct {
	paths    []string
	auth     string
	posted   map[string]any
	emailErr int // если не 0, резолв email отвечает этим кодом
}

func (f *fakeMM) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.paths = append(f.paths, r.Method+" "+r.URL.Path)
		f.auth = r.Header.Get("Authorization")
		switch {
		case r.URL.Path == "/api/v4/users/me":
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
			_ = json.NewDecoder(r.Body).Decode(&f.posted)
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
```

- [ ] **Шаг 2: Прогнать тест и убедиться, что он падает**

Запустить: `go test ./notifychannel/mattermost/ -v`
Ожидается: FAIL — пакета нет. Живой Mattermost не нужен: всё против `httptest`.

- [ ] **Шаг 3: Реализовать канал**

`notifychannel/mattermost/mattermost.go`:

```go
// Package mattermost delivers notifications as Mattermost direct messages.
//
// It addresses by email, so it needs no account-linking step and does not
// implement notifychannel.Linker. Public on purpose: channels are wired through
// app.Config.NotificationChannels next to main, and this package doubles as the
// worked example for a channel written in another repository.
package mattermost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"okrs/notifychannel"
)

// permanentError marks a failure that retrying cannot fix — an addressee with no
// Mattermost account, a malformed request. The delivery worker uses this to stop
// retrying instead of burning six attempts on a certainty.
type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

// IsPermanent reports whether the failure is worth retrying.
func IsPermanent(err error) bool {
	var p permanentError
	return errors.As(err, &p)
}

func permanent(format string, args ...any) error {
	return permanentError{err: fmt.Errorf(format, args...)}
}

// Channel returns the wiring unit to pass to app.Config.NotificationChannels.
func Channel() notifychannel.Channel {
	return notifychannel.Channel{
		Descriptor: notifychannel.Descriptor{
			Name:        "mattermost",
			Title:       "Mattermost",
			SecretField: "token",
			Fields: []notifychannel.Field{
				{
					Key: "base_url", Label: "Адрес сервера", Required: true,
					Kind: notifychannel.FieldURL,
					Hint: "Например https://mattermost.example.com — без завершающего слэша",
				},
				{
					Key: "token", Label: "Токен бота", Required: true,
					Kind: notifychannel.FieldSecret,
					Hint: "Personal Access Token бота. Боту нужны права на создание личных сообщений",
				},
			},
		},
		New: newSender,
	}
}

type sender struct {
	baseURL string
	token   string
	http    *http.Client

	// botID is resolved once and reused: the delivery worker sends in batches, and
	// re-asking who we are on every message is an N+1 over the network.
	once  sync.Once
	botID string
	botErr error
}

func newSender(s notifychannel.Settings) (notifychannel.Sender, error) {
	raw, _ := s.Values["base_url"].(string)
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return nil, errors.New("mattermost: base_url is required")
	}
	if _, err := url.ParseRequestURI(raw); err != nil {
		return nil, fmt.Errorf("mattermost: base_url is not a valid URL: %w", err)
	}
	if s.Secret == "" {
		return nil, notifychannel.ErrMissingSecret
	}
	return &sender{
		baseURL: raw,
		token:   s.Secret,
		http:    &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (s *sender) Send(ctx context.Context, target notifychannel.Target, msg notifychannel.Message) error {
	if target.Email == "" {
		return permanent("mattermost: no email to address")
	}
	botID, err := s.resolveBotID(ctx)
	if err != nil {
		return err
	}
	var user struct{ ID string `json:"id"` }
	if err := s.call(ctx, http.MethodGet, "/api/v4/users/email/"+url.PathEscape(target.Email), nil, &user); err != nil {
		return err
	}
	var dm struct{ ID string `json:"id"` }
	if err := s.call(ctx, http.MethodPost, "/api/v4/channels/direct", []string{botID, user.ID}, &dm); err != nil {
		return err
	}
	body := map[string]any{"channel_id": dm.ID, "message": format(msg)}
	return s.call(ctx, http.MethodPost, "/api/v4/posts", body, nil)
}

func (s *sender) resolveBotID(ctx context.Context) (string, error) {
	s.once.Do(func() {
		var me struct{ ID string `json:"id"` }
		if err := s.call(ctx, http.MethodGet, "/api/v4/users/me", nil, &me); err != nil {
			s.botErr = err
			return
		}
		s.botID = me.ID
	})
	return s.botID, s.botErr
}

// format renders the message as Markdown: bold title, body, then the link.
// The core already produced the wording; this only adds Mattermost's syntax.
func format(m notifychannel.Message) string {
	var b strings.Builder
	b.WriteString("**")
	b.WriteString(m.Title)
	b.WriteString("**")
	if m.Body != "" {
		b.WriteString("\n")
		b.WriteString(m.Body)
	}
	if m.URL != "" {
		b.WriteString("\n")
		b.WriteString(m.URL)
	}
	return b.String()
}

func (s *sender) call(ctx context.Context, method, path string, in, out any) error {
	var body *bytes.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return permanent("mattermost: encode %s: %v", path, err)
		}
		body = bytes.NewReader(raw)
	} else {
		body = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, body)
	if err != nil {
		return permanent("mattermost: build request %s: %v", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.http.Do(req)
	if err != nil {
		// Network failures are transient by nature — the worker should retry.
		return fmt.Errorf("mattermost: %s: %w", path, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		if out != nil {
			if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
				return fmt.Errorf("mattermost: decode %s: %w", path, err)
			}
		}
		return nil
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		// Rate limiting and server errors are worth retrying.
		return fmt.Errorf("mattermost: %s: status %d", path, resp.StatusCode)
	default:
		// 4xx: a missing addressee, a revoked token, a bad request. Retrying the
		// same call cannot change the outcome.
		return permanent("mattermost: %s: status %d", path, resp.StatusCode)
	}
}
```

- [ ] **Шаг 4: Прогнать тесты**

Запустить: `go test ./notifychannel/... -count=1 -v && gofmt -l notifychannel/`
Ожидается: PASS, семь тестов канала плюс три теста контракта; `gofmt` молчит.

- [ ] **Шаг 5: Проверить, что канал не тянет internal**

Запустить: `go list -deps ./notifychannel/mattermost/ | rg 'okrs/internal'`
Ожидается: пустой вывод. Канал зависит только от `okrs/notifychannel` и стандартной библиотеки — ровно то, что доступно автору из чужого модуля.


---

## Task 6: Опция сборки, проводка и системная панель

Здесь сейм становится настоящим: канал подключается из `main`, а системный администратор получает место, где выдать его пространству.

**Файлы:**
- Изменить: `internal/store/store.go` (поле `NotificationChannels`)
- Изменить: `internal/http/server.go` (`Options`, сборка сервиса, регистрация маршрута)
- Изменить: `app/app.go` (`Config.NotificationChannels`, `Config.NotificationSecretKey`)
- Изменить: `cmd/server/main.go` (чтение env, подключение Mattermost)
- Создать: `internal/http/handlers/api/v1/system/notificationchannels/handler.go`
- Изменить: `web/static/system.js` (раздел «Уведомления»)
- Тест: `internal/http/handlers/api/v1/system/notificationchannels/handler_test.go`
- Обновить: `internal/http/testdata/routes.golden`

**Интерфейсы:**
- Потребляет: `notificationchannel.Service` (задача 4), `mattermost.Channel()` (задача 5), `notificationchannels.Repository` (задача 3).
- Производит:
  - `app.Config.NotificationChannels []notifychannel.Channel`, `app.Config.NotificationSecretKey string`
  - `httpserver.Options.NotificationChannels`, `httpserver.Options.NotificationSecretKey`
  - `(*httpserver.Server).notifChannels *notificationchannel.Service` (неэкспортируемое поле)
  - `GET /api/v1/system/notification-channels` → `{"channels":[{"name","title","entitlement_key"}]}`
- Используют: задача 7 (админка пространства), фаза 2a-2 (воркер доставки).

**Соглашения репозитория, которым здесь надо следовать буквально** (проверено по коду, не по памяти):
- Системные хендлеры пишут ответ через `systemcommon.WriteJSON(w, v)` — **два аргумента**, статус ставится отдельно через `w.WriteHeader`, и через `systemcommon.WriteError(w, status, msg)` — **три аргумента, тело `{"error":"текст"}` плоской строкой**. Машинных кодов ошибок в этих двух плоскостях нет; `handlertest.ErrorCode` рассчитан на другой формат и здесь неприменим.
- `handlertest.Do(h http.HandlerFunc, method, target, body string, opts ...Option)` — **без `t` первым аргументом**, тело — строка (пустая, а не nil).
- Гейт системной плоскости — `auth.RequireSystemAdminMiddleware` на группе маршрутов в `registerSystemRoutes`. Хендлер роль не проверяет: во всей системной плоскости это делает роутер, и второй способ проверки развёл бы две несогласуемые правды.

**Почему сервис собирается в `NewServer`, а не в `httpdeps.Build`.** `Build` — чистая сборка графа сервисов из стора и шины; ему не передают ни `Options`, ни `entitlements`. Каналы зависят и от того, и от другого. Тянуть три новых параметра в `Build` ради одного сервиса — хуже, чем собрать его там, где обе зависимости уже под рукой.

**Почему у системной панели нет своего эндпоинта записи.** Разрешение канала — это обычный entitlement, и он уже пишется через `PUT /api/v1/system/tenants/{id}/entitlements`. Новый маршрут нужен ровно один и только на чтение: узнать, какие каналы вообще есть в этой сборке. Ключ, который панель отправляет, — **голый** `notifications.mattermost`; `SetEntitlements` добавит префикс сам (`internal/service/provisioning/provisioning.go`).

- [ ] **Шаг 1: Написать падающий тест хендлера**

`internal/http/handlers/api/v1/system/notificationchannels/handler_test.go`:

```go
package notificationchannels_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"okrs/internal/http/handlers/api/v1/system/notificationchannels"
	"okrs/internal/http/handlers/handlertest"
	"okrs/notifychannel"
	"okrs/notifychannel/mattermost"
)

type fakeSvc struct{ ds []notifychannel.Descriptor }

func (f fakeSvc) Descriptors() []notifychannel.Descriptor { return f.ds }

// Панель выдаёт каналы пространствам, поэтому ей нужен список того, что есть в
// сборке, — вместе с готовым ключом entitlement, чтобы не собирать его в JS.
func TestListReturnsBuildChannelsWithEntitlementKeys(t *testing.T) {
	h := notificationchannels.New(fakeSvc{ds: []notifychannel.Descriptor{
		mattermost.Channel().Descriptor,
	}})

	rec := handlertest.Do(h.List, http.MethodGet, "/api/v1/system/notification-channels", "")
	handlertest.Status(t, rec, http.StatusOK)

	var got struct {
		Channels []struct {
			Name           string `json:"name"`
			Title          string `json:"title"`
			EntitlementKey string `json:"entitlement_key"`
		} `json:"channels"`
	}
	handlertest.DecodeJSON(t, rec, &got)
	if len(got.Channels) != 1 {
		t.Fatalf("каналы: %+v", got.Channels)
	}
	c := got.Channels[0]
	if c.Name != "mattermost" || c.Title != "Mattermost" {
		t.Fatalf("канал: %+v", c)
	}
	// Голый ключ, без префикса: SetEntitlements добавит "entitlement." сам, и
	// панель, отправив полный ключ, получила бы "entitlement.entitlement.…".
	if c.EntitlementKey != "notifications.mattermost" {
		t.Fatalf("ключ entitlement: %q", c.EntitlementKey)
	}
}

// Сборка без каналов отвечает пустым массивом, а не null: JS итерирует поле.
func TestListWithNoChannelsReturnsEmptyArray(t *testing.T) {
	h := notificationchannels.New(fakeSvc{})
	rec := handlertest.Do(h.List, http.MethodGet, "/api/v1/system/notification-channels", "")
	handlertest.Status(t, rec, http.StatusOK)

	var got struct {
		Channels []json.RawMessage `json:"channels"`
	}
	handlertest.DecodeJSON(t, rec, &got)
	if got.Channels == nil {
		t.Fatalf("ожидался пустой массив, а не null: %s", handlertest.Body(rec))
	}
	if len(got.Channels) != 0 {
		t.Fatalf("каналы: %s", handlertest.Body(rec))
	}
}
```

- [ ] **Шаг 2: Прогнать тест и убедиться, что он падает**

Запустить: `go test ./internal/http/handlers/api/v1/system/notificationchannels/ -v`
Ожидается: FAIL — пакета нет.

- [ ] **Шаг 3: Написать хендлер**

`internal/http/handlers/api/v1/system/notificationchannels/handler.go`:

```go
// Package notificationchannels exposes, to the system-admin panel, the notification
// channels this build contains. Read-only on purpose: granting a channel to a tenant
// is an ordinary entitlement write and already has an endpoint. What the panel cannot
// know on its own is which channels the binary was assembled with — a build may carry
// channels from another repository entirely.
//
// Access is gated by auth.RequireSystemAdminMiddleware on the route group, as
// everywhere else in the system plane; there is no role check here.
package notificationchannels

import (
	"net/http"

	"okrs/internal/http/handlers/api/v1/system/systemcommon"
	"okrs/notifychannel"

	"github.com/go-chi/chi/v5"
)

// Channels is the port: only the build list, nothing tenant-specific.
type Channels interface {
	Descriptors() []notifychannel.Descriptor
}

type Handler struct{ svc Channels }

func New(svc Channels) *Handler { return &Handler{svc: svc} }

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/api/v1/system/notification-channels", h.List)
}

type channelDTO struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	// EntitlementKey is the BARE key the panel must send to the entitlements
	// endpoint. provisioning.SetEntitlements prefixes it with "entitlement.", so
	// sending the full key would store "entitlement.entitlement.notifications.…".
	EntitlementKey string `json:"entitlement_key"`
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ds := h.svc.Descriptors()
	// Non-nil slice: the panel iterates the field, and null would need a guard in JS.
	out := make([]channelDTO, 0, len(ds))
	for _, d := range ds {
		out = append(out, channelDTO{
			Name:           d.Name,
			Title:          d.Title,
			EntitlementKey: "notifications." + d.Name,
		})
	}
	systemcommon.WriteJSON(w, map[string]any{"channels": out})
}
```

- [ ] **Шаг 4: Прогнать тест хендлера**

Запустить: `go test ./internal/http/handlers/api/v1/system/notificationchannels/ -count=1 -v`
Ожидается: PASS, два теста.

- [ ] **Шаг 5: Проверить, что репозиторий уже заведён в сторе**

Проводка `internal/store/store.go` (поле `NotificationChannels`, инициализация, импорт) выполнена в задаче 3 — план ошибочно содержал этот шаг дважды (Ruling 5). Здесь ничего писать не нужно, только убедиться.

Запустить: `rg -n 'NotificationChannels' internal/store/store.go`
Ожидается: две строки — объявление поля и `notificationchannels.NewRepository(db)` в конструкторе. Если их нет — добавить по образцу соседних полей уведомлений и сообщить об этом в отчёте.

- [ ] **Шаг 6: Собрать сервис в `NewServer` и зарегистрировать маршрут**

В `internal/http/server.go`:

1. В `Options` — два поля:

```go
	// NotificationChannels are the delivery channels compiled into this build. Empty
	// in the plain OSS box: in-app needs no channel. A build assembles them next to
	// main (see app.Config), which is what lets a channel live in another module.
	NotificationChannels []notifychannel.Channel
	// NotificationSecretKey is the base64 32-byte key used to encrypt channel
	// secrets at rest. Empty means channels with secrets cannot be configured; the
	// rest of the application, in-app notifications included, works unchanged.
	NotificationSecretKey string
```

2. В структуре `Server` — поле `notifChannels *notificationchannel.Service`.

3. В `NewServer`, после того как определён `ent` (около строки 248):

```go
	// A missing key must not take the whole application down: a box that cannot
	// encrypt channel secrets still serves in-app notifications and everything else.
	// A key that is present but malformed is a different matter — that is an operator
	// error which has to be seen, so it fails startup.
	var secrets *secretbox.Box
	if opts.NotificationSecretKey != "" {
		var err error
		secrets, err = secretbox.New(opts.NotificationSecretKey)
		if err != nil {
			return nil, fmt.Errorf("http: notification secret key: %w", err)
		}
	} else if len(opts.NotificationChannels) > 0 {
		logger.Warn("notification channels are compiled in but NOTIFICATIONS_SECRET_KEY is unset; channels requiring a secret cannot be configured")
	}
	notifChannels, err := notificationchannel.New(st.NotificationChannels, secrets, opts.NotificationChannels, ent)
	if err != nil {
		return nil, fmt.Errorf("http: notification channels: %w", err)
	}
```
и `notifChannels: notifChannels` в литерале `&Server{…}`.

> Если в этой точке `err` ещё не объявлена в области видимости — использовать `:=` для пары `notifChannels, err`; при реализации свериться с фактическим кодом функции, а не подгонять его под этот фрагмент.

4. В `registerSystemRoutes`, рядом с `sysentitlements.RegisterRoutes`:

```go
		sysnotifchan.RegisterRoutes(r, sysnotifchan.New(s.notifChannels))
```
с импортом `sysnotifchan "okrs/internal/http/handlers/api/v1/system/notificationchannels"`.

- [ ] **Шаг 7: Пробросить опцию через `app.Config`**

В `app/app.go` — два поля `Config` рядом с остальными сеймами:

```go
	// NotificationChannels are the delivery channels this build offers. Assembled by
	// the caller, so a private module can add its own without touching this one; the
	// OSS box passes the channels in cmd/server. Nil means in-app only.
	NotificationChannels []notifychannel.Channel
	// NotificationSecretKey (base64, 32 bytes) encrypts channel secrets at rest.
	NotificationSecretKey string
```
и в вызове `httpserver.NewServer(...)`:
```go
		NotificationChannels:  cfg.NotificationChannels,
		NotificationSecretKey: cfg.NotificationSecretKey,
```

- [ ] **Шаг 8: Подключить Mattermost в `cmd/server/main.go`**

Рядом с заполнением остальных полей `app.Config`:

```go
	// Channels are assembled here, next to main, and not registered from a package
	// init: that is the point of the seam — another build can swap this list for its
	// own without the application knowing the channel's package exists.
	cfg.NotificationChannels = []notifychannel.Channel{mattermost.Channel()}
	cfg.NotificationSecretKey = os.Getenv("NOTIFICATIONS_SECRET_KEY")
```
с импортами `"okrs/notifychannel"` и `"okrs/notifychannel/mattermost"`.

- [ ] **Шаг 9: Раздел «Уведомления» в системной панели**

В `web/static/system.js` добавить в `SYSTEM_SECTIONS` после `entitlements`:

```js
  {id:'notifications', label:'Уведомления',  icon:'🔔'},
```

Компонент — рядом с `EntitlementsSection`, в тех же стилях (`box`, `inp`, `btn`, `C`) и на тех же хелперах (`get`, `put`, `errMsg`):

```jsx
// Каналы уведомлений: список берётся из сборки, а выдача пространству — это
// обычный entitlement. Отдельного эндпоинта записи здесь нет намеренно: он бы
// дублировал /entitlements и дал второй путь к тем же данным.
function NotificationChannelsSection({tenants}) {
  const [channels,setChannels]=useState([]);
  const [tid,setTid]=useState(''); const [ent,setEnt]=useState({}); const [msg,setMsg]=useState('');
  useEffect(()=>{ (async()=>{ const r=await get('/api/v1/system/notification-channels'); if(r&&r.ok){ const j=await r.json(); setChannels(j.channels||[]); } })(); },[]);
  const load = useCallback(async(id)=>{ if(!id){setEnt({});return;} const r=await get(`/api/v1/system/tenants/${id}/entitlements`); if(r&&r.ok) setEnt(await r.json()||{}); },[]);
  useEffect(()=>{ load(tid); },[tid,load]);
  const toggle = async (key, on)=>{ setMsg(''); const r=await put(`/api/v1/system/tenants/${tid}/entitlements`, {[key]: on}); if(r&&r.status===204){ load(tid); setMsg('Сохранено'); } else setMsg(await errMsg(r)); };
  return <div style={box}>
    <h2 style={{fontSize:15,marginBottom:6}}>Каналы уведомлений</h2>
    <div style={{color:C.muted,marginBottom:10}}>
      Список каналов задаётся сборкой. Здесь выбирается, какие из них доступны пространству;
      подключает и настраивает канал уже администратор пространства. Внутренние уведомления
      (колокольчик) доступны всегда и в этот список не входят.
    </div>
    {!channels.length && <div style={{color:C.muted}}>В этой сборке нет внешних каналов.</div>}
    {!!channels.length && <select style={inp} value={tid} onChange={e=>setTid(e.target.value)}>
      <option value="">— выберите пространство —</option>
      {(tenants||[]).map(t=><option key={t.id} value={t.id}>{t.name} ({t.slug})</option>)}
    </select>}
    {tid && <div style={{marginTop:12,display:'flex',flexDirection:'column',gap:8}}>
      {channels.map(ch=>{
        const on = ent[ch.entitlement_key]===true;
        return <div key={ch.name} style={{display:'flex',gap:8,alignItems:'center'}}>
          <span style={{minWidth:160,fontWeight:600}}>{ch.title}</span>
          <code style={{minWidth:220,color:C.muted}}>entitlement.{ch.entitlement_key}</code>
          <button style={{...btn,background: on?C.ok:C.muted}} onClick={()=>toggle(ch.entitlement_key, !on)}>
            {on?'доступен':'выключен'}
          </button>
        </div>;
      })}
      {msg && <div style={{color:msg==='Сохранено'?C.ok:C.danger}}>{msg}</div>}
    </div>}
  </div>;
}
```

и рендер рядом с остальными разделами:
```jsx
    {section==='notifications' && <NotificationChannelsSection tenants={tenants}/>}
```

- [ ] **Шаг 10: Обновить golden маршрутов**

Запустить: `go test ./internal/http -run TestRoutesGolden -update-routes && go test ./internal/http -count=1`
Ожидается: PASS. В `internal/http/testdata/routes.golden` появляется одна строка — `GET /api/v1/system/notification-channels`. Файл в дереве хранится с CRLF, и флаг записывает его обратно с CRLF (правка задачи 6 фазы 1b) — `git diff --stat` должен показать одну добавленную строку, а не переписанный целиком файл.

- [ ] **Шаг 11: Прогнать сборку целиком**

Запустить: `go build ./... && go vet ./... && go test ./... -count=1`
Ожидается: PASS. Если падает `TestSpecRouteTableMatchesRouter` — не подгонять его под код: таблица маршрутов в `specs/040` правится в задаче 8, и если тест сверяется со спекой, эта задача завершится с известным красным тестом, который закроет задача 8. Зафиксировать это в отчёте, а не «починить» удалением проверки.

- [ ] **Шаг 12: Проверить, что JS парсится**

Запустить: `node -e "const B=require('./web/static/vendor/babel.min.js');const fs=require('fs');B.transform(fs.readFileSync('web/static/system.js','utf8'),{presets:['react']});console.log('ok')"`
Ожидается: `ok`. Babel вендорится в репозитории — сеть для этой проверки не нужна.

- [ ] **Шаг 13: Убедиться, что тест кусается**

Мутация изолирует ровно одно свойство — формат ключа entitlement.

```bash
mkdir -p /tmp/mut && sed 's|"notifications\." + d.Name|"entitlement.notifications." + d.Name|' \
  internal/http/handlers/api/v1/system/notificationchannels/handler.go > /tmp/mut/handler.go
printf '{"Replace":{"%s":"%s"}}' \
  "$PWD/internal/http/handlers/api/v1/system/notificationchannels/handler.go" "/tmp/mut/handler.go" > /tmp/mut/overlay.json
go test -overlay=/tmp/mut/overlay.json ./internal/http/handlers/api/v1/system/notificationchannels/ -count=1
```
Ожидается: FAIL в `TestListReturnsBuildChannelsWithEntitlementKeys`. Сборка при этом обязана быть успешной — падение компиляции ничего не доказывает.

---

## Task 7: Экран администратора пространства

Администратор пространства видит только выданные ему каналы, настраивает их и может проверить связь до того, как на канал пойдут настоящие уведомления.

**Файлы:**
- Создать: `internal/http/dto/notificationchannel.go`
- Создать: `internal/http/handlers/api/v1/admin/settings/notifications/handler.go`
- Создать: `internal/http/handlers/api/v1/admin/settings/notifications/test/handler.go`
- Изменить: `internal/http/server.go` (регистрация двух пакетов)
- Изменить: `web/static/admin.js` (раздел «Уведомления»)
- Тесты: `.../notifications/handler_test.go`, `.../notifications/test/handler_test.go`
- Обновить: `internal/http/testdata/routes.golden`

**Интерфейсы:**
- Потребляет: `notificationchannel.Service` (задача 4).
- Производит:
  - `GET /api/v1/admin/settings/notifications` → `{"channels":[{name,title,enabled,configured,secret_hint,values,fields}]}`
  - `PUT /api/v1/admin/settings/notifications/{channel}` ← `{enabled,values,secret}` → 204
  - `POST /api/v1/admin/settings/notifications/{channel}/test` → `{"ok":true}` либо ошибка
- Используют: фаза 2a-2 (колонки каналов в пользовательских настройках).

**Соглашения репозитория, проверенные по коду:**
- Admin-хендлеры пишут через `admincommon.WriteJSON(w, v)` (два аргумента) и `admincommon.WriteError(w, status, msg)` (три аргумента, тело `{"error":"текст"}`). Машинных кодов ошибок здесь нет — тесты проверяют статус и текст.
- Скоуп берётся `auth.TenantScopeFromContext(ctx) (domain.TenantScope, bool)`; идентификатор вызывающего — `auth.UserIDFromContext(ctx) int64`; сам пользователь — `auth.UserFromContext(ctx) *domain.User` (у него есть `Email`).
- Роль **не проверяется в хендлере**: группу `registerAdminRoutes` уже закрывает `auth.RequireTenantAdminMiddleware`. Так устроены все соседние admin-хендлеры (`settings/access`, `settings/feedback`), и второй способ проверки развёл бы две правды. Соответственно и теста «участника не пускают» на уровне хендлера нет — вместо него шаг 9 проверяет, что маршруты зарегистрированы внутри защищённой группы.
- `handlertest.Do(h, method, target, body string, opts ...Option)` — без `t`, тело строкой; `handlertest.UserID(id int64, udid string)` — **два аргумента**.

**Почему пакет на URI-сегмент.** Правило `specs/070`: `/api/v1/admin/settings/notifications` и `.../{channel}/test` — разные пакеты. Проверка связи меняет состояние внешней системы (отправляет сообщение), поэтому `POST` и, как следствие, CSRF.

**Кому уходит тестовое сообщение.** Тому, кто нажал кнопку, — и никому больше. Слать его кому-то ещё значит из чужой формы класть сообщение в чужой мессенджер. Адрес берётся из `auth.UserFromContext`, запроса к базе для этого не нужно.

- [ ] **Шаг 1: Написать падающий тест чтения и записи**

`internal/http/handlers/api/v1/admin/settings/notifications/handler_test.go`:

```go
package notifications_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"okrs/internal/core/domain"
	adminnotifications "okrs/internal/http/handlers/api/v1/admin/settings/notifications"
	"okrs/internal/http/handlers/handlertest"
	notificationchannelsvc "okrs/internal/service/notificationchannel"
	"okrs/notifychannel"
)

type fakeSvc struct {
	states  []notificationchannelsvc.ChannelState
	saved   notificationchannelsvc.SaveInput
	byUser  int64
	saveErr error
}

func (f *fakeSvc) List(context.Context, domain.TenantScope) ([]notificationchannelsvc.ChannelState, error) {
	return f.states, nil
}

func (f *fakeSvc) Save(_ context.Context, _ domain.TenantScope, in notificationchannelsvc.SaveInput, by int64) error {
	f.saved, f.byUser = in, by
	return f.saveErr
}

func state() notificationchannelsvc.ChannelState {
	return notificationchannelsvc.ChannelState{
		Descriptor: notifychannel.Descriptor{
			Name: "mattermost", Title: "Mattermost", SecretField: "token",
			Fields: []notifychannel.Field{
				{Key: "base_url", Label: "Адрес сервера", Required: true, Kind: notifychannel.FieldURL},
				{Key: "token", Label: "Токен бота", Required: true, Kind: notifychannel.FieldSecret},
			},
		},
		Enabled: true, Configured: true,
		Values:     map[string]any{"base_url": "https://mm.example.com"},
		SecretHint: "••••4821",
	}
}

// Форма рисуется по дескриптору, а секрет уходит клиенту только маской.
func TestListDescribesFormAndMasksSecret(t *testing.T) {
	h := adminnotifications.New(&fakeSvc{states: []notificationchannelsvc.ChannelState{state()}})
	rec := handlertest.Do(h.List, http.MethodGet, "/api/v1/admin/settings/notifications", "",
		handlertest.Tenant(1))
	handlertest.Status(t, rec, http.StatusOK)

	var got struct {
		Channels []struct {
			Name       string           `json:"name"`
			Enabled    bool             `json:"enabled"`
			SecretHint string           `json:"secret_hint"`
			Values     map[string]any   `json:"values"`
			Fields     []map[string]any `json:"fields"`
		} `json:"channels"`
	}
	handlertest.DecodeJSON(t, rec, &got)
	if len(got.Channels) != 1 {
		t.Fatalf("каналы: %+v", got.Channels)
	}
	c := got.Channels[0]
	if c.Name != "mattermost" || !c.Enabled || c.SecretHint != "••••4821" {
		t.Fatalf("канал: %+v", c)
	}
	if len(c.Fields) != 2 {
		t.Fatalf("поля формы: %+v", c.Fields)
	}
	if c.Values["token"] != nil {
		t.Fatal("секретное поле попало в values")
	}
}

// Отдельная проверка утечки: даже если реализация начнёт возвращать значения без
// санитайза, тело ответа не должно содержать сам секрет.
func TestListNeverEchoesPlaintextSecret(t *testing.T) {
	const secret = "token-abcdef4821"
	st := state()
	// Секрет, «случайно» попавший в несекретные значения — ровно тот сценарий,
	// от которого защищает санитайз в сервисе и в хендлере.
	st.Values = map[string]any{"base_url": "https://mm.example.com", "token": secret}
	h := adminnotifications.New(&fakeSvc{states: []notificationchannelsvc.ChannelState{st}})

	rec := handlertest.Do(h.List, http.MethodGet, "/api/v1/admin/settings/notifications", "",
		handlertest.Tenant(1))
	handlertest.Status(t, rec, http.StatusOK)
	if strings.Contains(handlertest.Body(rec), secret) {
		t.Fatalf("секрет в теле ответа: %s", handlertest.Body(rec))
	}
}

// PUT доносит до сервиса значения, секрет и автора правки.
func TestSavePassesInputThrough(t *testing.T) {
	svc := &fakeSvc{states: []notificationchannelsvc.ChannelState{state()}}
	h := adminnotifications.New(svc)
	body := `{"enabled":true,"values":{"base_url":"https://mm2"},"secret":"новый-токен"}`
	rec := handlertest.Do(h.Save, http.MethodPut,
		"/api/v1/admin/settings/notifications/mattermost", body,
		handlertest.Tenant(1), handlertest.UserID(42, "udid-42"),
		handlertest.URLParam("channel", "mattermost"))
	handlertest.Status(t, rec, http.StatusNoContent)

	if svc.saved.Channel != "mattermost" || svc.saved.Secret != "новый-токен" {
		t.Fatalf("вход сервиса: %+v", svc.saved)
	}
	if svc.saved.Values["base_url"] != "https://mm2" {
		t.Fatalf("значения: %+v", svc.saved.Values)
	}
	if svc.byUser != 42 {
		t.Fatalf("автор правки: %d", svc.byUser)
	}
}

// Канал, не выданный пространству, — 404, а не 403: подтверждать существование
// канала, которого у пространства нет, значит показывать чужой каталог.
// Дизайн-спека §13.4: недоступные каналы не показываем вовсе.
func TestSaveOfUnavailableChannelIs404(t *testing.T) {
	svc := &fakeSvc{saveErr: notificationchannelsvc.ErrNotAvailable}
	h := adminnotifications.New(svc)
	rec := handlertest.Do(h.Save, http.MethodPut,
		"/api/v1/admin/settings/notifications/telegram", `{"enabled":true}`,
		handlertest.Tenant(1), handlertest.UserID(42, "udid-42"),
		handlertest.URLParam("channel", "telegram"))
	handlertest.Status(t, rec, http.StatusNotFound)
}

// Неизвестный канал отвечает так же, как недоступный: различие в ответах дало бы
// перебором полный список каналов продукта.
func TestSaveOfUnknownChannelIs404(t *testing.T) {
	h := adminnotifications.New(&fakeSvc{saveErr: notificationchannelsvc.ErrUnknownChannel})
	rec := handlertest.Do(h.Save, http.MethodPut,
		"/api/v1/admin/settings/notifications/nope", `{"enabled":true}`,
		handlertest.Tenant(1), handlertest.UserID(42, "udid-42"),
		handlertest.URLParam("channel", "nope"))
	handlertest.Status(t, rec, http.StatusNotFound)
}

// Отсутствие ключа шифрования — ошибка развёртывания, и администратор должен
// прочитать её как таковую, а не как «что-то пошло не так».
func TestSaveWithoutSecretKeyExplainsItself(t *testing.T) {
	h := adminnotifications.New(&fakeSvc{saveErr: notificationchannelsvc.ErrNoSecretKey})
	rec := handlertest.Do(h.Save, http.MethodPut,
		"/api/v1/admin/settings/notifications/mattermost", `{"enabled":true,"secret":"x"}`,
		handlertest.Tenant(1), handlertest.UserID(42, "udid-42"),
		handlertest.URLParam("channel", "mattermost"))
	handlertest.Status(t, rec, http.StatusServiceUnavailable)
	if !strings.Contains(handlertest.Body(rec), "NOTIFICATIONS_SECRET_KEY") {
		t.Fatalf("сообщение не называет причину: %s", handlertest.Body(rec))
	}
}

// Без активного пространства настройки недоступны.
func TestRequiresTenantScope(t *testing.T) {
	h := adminnotifications.New(&fakeSvc{states: []notificationchannelsvc.ChannelState{state()}})
	handlertest.RequiresTenantScope(t, h.List, http.MethodGet, "/api/v1/admin/settings/notifications")
}
```

`internal/http/handlers/api/v1/admin/settings/notifications/test/handler_test.go`:

```go
package test_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"okrs/internal/core/domain"
	channeltest "okrs/internal/http/handlers/api/v1/admin/settings/notifications/test"
	"okrs/internal/http/handlers/handlertest"
	notificationchannelsvc "okrs/internal/service/notificationchannel"
	"okrs/notifychannel"
)

type fakeSender struct {
	target notifychannel.Target
	msg    notifychannel.Message
	err    error
}

func (f *fakeSender) Send(_ context.Context, tg notifychannel.Target, m notifychannel.Message) error {
	f.target, f.msg = tg, m
	return f.err
}

type fakeSvc struct {
	sender *fakeSender
	err    error
}

func (f *fakeSvc) Sender(context.Context, domain.TenantScope, string) (notifychannel.Sender, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.sender, nil
}

// Тестовое сообщение уходит тому, кто нажал кнопку, — и никому больше.
func TestSendsToTheCallerOnly(t *testing.T) {
	s := &fakeSender{}
	h := channeltest.New(&fakeSvc{sender: s})
	rec := handlertest.Do(h.Test, http.MethodPost,
		"/api/v1/admin/settings/notifications/mattermost/test", "",
		handlertest.Tenant(1), handlertest.User("udid-42"),
		handlertest.URLParam("channel", "mattermost"))
	handlertest.Status(t, rec, http.StatusOK)

	if s.target.Email != "admin@example.com" {
		t.Fatalf("адресат: %+v", s.target)
	}
	if s.msg.Title == "" {
		t.Fatal("пустое тестовое сообщение")
	}
}

// Ненастроенный канал — 409, а не 500: администратору надо сначала сохранить
// настройки, и текст ошибки должен вести именно туда.
func TestUnconfiguredChannelIsConflict(t *testing.T) {
	h := channeltest.New(&fakeSvc{err: notificationchannelsvc.ErrNotConfigured})
	rec := handlertest.Do(h.Test, http.MethodPost,
		"/api/v1/admin/settings/notifications/mattermost/test", "",
		handlertest.Tenant(1), handlertest.User("udid-42"),
		handlertest.URLParam("channel", "mattermost"))
	handlertest.Status(t, rec, http.StatusConflict)
}

// Недоступный канал — 404, теми же соображениями, что и в соседнем пакете.
func TestUnavailableChannelIs404(t *testing.T) {
	h := channeltest.New(&fakeSvc{err: notificationchannelsvc.ErrNotAvailable})
	rec := handlertest.Do(h.Test, http.MethodPost,
		"/api/v1/admin/settings/notifications/telegram/test", "",
		handlertest.Tenant(1), handlertest.User("udid-42"),
		handlertest.URLParam("channel", "telegram"))
	handlertest.Status(t, rec, http.StatusNotFound)
}

// Ошибку доставки показываем как есть: это единственный способ для администратора
// узнать, что токен отозван или адрес сервера неверен.
func TestDeliveryFailureIsReportedToTheAdmin(t *testing.T) {
	h := channeltest.New(&fakeSvc{sender: &fakeSender{err: errors.New("mattermost: status 401")}})
	rec := handlertest.Do(h.Test, http.MethodPost,
		"/api/v1/admin/settings/notifications/mattermost/test", "",
		handlertest.Tenant(1), handlertest.User("udid-42"),
		handlertest.URLParam("channel", "mattermost"))
	handlertest.Status(t, rec, http.StatusBadGateway)
	if !strings.Contains(handlertest.Body(rec), "401") {
		t.Fatalf("ответ не объясняет причину: %s", handlertest.Body(rec))
	}
}

func TestRequiresTenantScope(t *testing.T) {
	h := channeltest.New(&fakeSvc{sender: &fakeSender{}})
	handlertest.RequiresTenantScope(t, h.Test, http.MethodPost,
		"/api/v1/admin/settings/notifications/mattermost/test",
		handlertest.URLParam("channel", "mattermost"))
}
```

> `handlertest.User("udid-42")` кладёт в контекст пользователя. Реализуя тест, проверить в `internal/http/handlers/handlertest/handlertest.go`, какой `Email` он проставляет: если хелпер оставляет email пустым, добавить в него параметр или собрать контекст в тесте вручную через `auth.WithUser`. Ожидание `admin@example.com` в тесте подогнать под то, что хелпер реально кладёт, — но **не** убирать саму проверку адресата: именно она доказывает, что сообщение не уходит третьему лицу. Отдельным тестом покрыть пользователя без email (ожидаемый статус `422`).

- [ ] **Шаг 2: Прогнать тесты и убедиться, что они падают**

Запустить: `go test ./internal/http/handlers/api/v1/admin/settings/notifications/... -v`
Ожидается: FAIL — пакетов нет.

- [ ] **Шаг 3: Написать DTO**

`internal/http/dto/notificationchannel.go`:

```go
package dto

// NotificationChannelField describes one input of a channel's configuration form.
// The admin screen renders from this and knows nothing about any specific channel —
// which is what lets a channel from another module get a working form for free.
type NotificationChannelField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Kind     string `json:"kind"` // text | url | secret
	Required bool   `json:"required"`
	Hint     string `json:"hint,omitempty"`
}

// NotificationChannelDTO is one channel as the tenant admin sees it.
//
// There is deliberately no field carrying the secret itself: SecretHint is a mask
// ("••••4821"), enough to tell "a token is saved" from "no token yet" and not enough
// to be one. Sending the plaintext back — even to an admin, even over TLS — would put
// it in browser memory, in devtools, and in anything that proxies the response.
type NotificationChannelDTO struct {
	Name       string                     `json:"name"`
	Title      string                     `json:"title"`
	Enabled    bool                       `json:"enabled"`
	Configured bool                       `json:"configured"`
	SecretHint string                     `json:"secret_hint,omitempty"`
	Values     map[string]any             `json:"values"`
	Fields     []NotificationChannelField `json:"fields"`
}
```

- [ ] **Шаг 4: Написать хендлер настроек**

`internal/http/handlers/api/v1/admin/settings/notifications/handler.go`:

```go
// Package notifications serves the tenant admin's notification-channel screen: which
// channels this tenant may use, what is configured, and how to change it.
//
// The screen only ever shows channels the tenant was granted (design spec §13.4) —
// no locked cards, no upsell. That filtering lives in the service; this package must
// not add a channel the service did not return.
//
// Admin rights are enforced by auth.RequireTenantAdminMiddleware on the route group,
// as in every neighbouring admin handler; there is no role check here.
package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/dto"
	"okrs/internal/http/handlers/api/v1/admin/admincommon"
	notificationchannelsvc "okrs/internal/service/notificationchannel"

	"github.com/go-chi/chi/v5"
)

// Channels is the port, declared consumer-side per specs/010.
type Channels interface {
	List(ctx context.Context, scope domain.TenantScope) ([]notificationchannelsvc.ChannelState, error)
	Save(ctx context.Context, scope domain.TenantScope, in notificationchannelsvc.SaveInput, byUserID int64) error
}

type Handler struct{ svc Channels }

func New(svc Channels) *Handler { return &Handler{svc: svc} }

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/api/v1/admin/settings/notifications", h.List)
	r.Put("/api/v1/admin/settings/notifications/{channel}", h.Save)
}

// GET /api/v1/admin/settings/notifications
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		admincommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	states, err := h.svc.List(r.Context(), scope)
	if err != nil {
		admincommon.WriteError(w, http.StatusInternalServerError, "не удалось получить каналы")
		return
	}

	out := make([]dto.NotificationChannelDTO, 0, len(states))
	for _, st := range states {
		fields := make([]dto.NotificationChannelField, 0, len(st.Descriptor.Fields))
		for _, f := range st.Descriptor.Fields {
			fields = append(fields, dto.NotificationChannelField{
				Key: f.Key, Label: f.Label, Kind: string(f.Kind), Required: f.Required, Hint: f.Hint,
			})
		}
		out = append(out, dto.NotificationChannelDTO{
			Name: st.Descriptor.Name, Title: st.Descriptor.Title,
			Enabled: st.Enabled, Configured: st.Configured, SecretHint: st.SecretHint,
			Values: publicValues(st),
			Fields: fields,
		})
	}
	admincommon.WriteJSON(w, map[string]any{"channels": out})
}

// publicValues drops the secret field a second time. The service already strips it,
// but this is the last hop before the wire: one guard at the boundary means a bug in
// the layer below cannot turn into a leak, and it costs one map copy per channel.
func publicValues(st notificationchannelsvc.ChannelState) map[string]any {
	out := make(map[string]any, len(st.Values))
	for k, v := range st.Values {
		if st.Descriptor.SecretField != "" && k == st.Descriptor.SecretField {
			continue
		}
		out[k] = v
	}
	return out
}

type saveRequest struct {
	Enabled bool           `json:"enabled"`
	Values  map[string]any `json:"values"`
	// Secret empty means "keep the stored one": the form shows a mask, and an admin
	// editing only the server URL must not silently drop the token.
	Secret string `json:"secret"`
}

// PUT /api/v1/admin/settings/notifications/{channel}
func (h *Handler) Save(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		admincommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	var req saveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		admincommon.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}

	in := notificationchannelsvc.SaveInput{
		Channel: chi.URLParam(r, "channel"),
		Enabled: req.Enabled,
		Values:  req.Values,
		Secret:  req.Secret,
	}
	err := h.svc.Save(r.Context(), scope, in, auth.UserIDFromContext(r.Context()))
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, notificationchannelsvc.ErrUnknownChannel),
		errors.Is(err, notificationchannelsvc.ErrNotAvailable):
		// One status for both: a channel this tenant does not have must be
		// indistinguishable from one that does not exist. Answering 403 to the former
		// would confirm which channels the product has — an admin is not supposed to
		// enumerate a catalogue they were not granted.
		admincommon.WriteError(w, http.StatusNotFound, "канал недоступен")
	case errors.Is(err, notificationchannelsvc.ErrNoSecretKey):
		admincommon.WriteError(w, http.StatusServiceUnavailable,
			"на сервере не настроен ключ шифрования NOTIFICATIONS_SECRET_KEY — секрет канала сохранить нельзя")
	default:
		admincommon.WriteError(w, http.StatusInternalServerError, "не удалось сохранить канал")
	}
}
```

- [ ] **Шаг 5: Написать хендлер проверки**

`internal/http/handlers/api/v1/admin/settings/notifications/test/handler.go`:

```go
// Package test sends one probe message through a configured channel so the admin
// learns the settings are wrong now, at the form, rather than from users who never
// received anything.
//
// The probe goes to the caller and to nobody else: sending it elsewhere would let one
// admin's form put messages into another person's messenger.
package test

import (
	"context"
	"errors"
	"net/http"

	"okrs/internal/auth"
	"okrs/internal/core/domain"
	"okrs/internal/http/handlers/api/v1/admin/admincommon"
	notificationchannelsvc "okrs/internal/service/notificationchannel"
	"okrs/notifychannel"

	"github.com/go-chi/chi/v5"
)

type Channels interface {
	Sender(ctx context.Context, scope domain.TenantScope, name string) (notifychannel.Sender, error)
}

type Handler struct{ svc Channels }

func New(svc Channels) *Handler { return &Handler{svc: svc} }

func RegisterRoutes(r chi.Router, h *Handler) {
	// POST, not GET: this changes state in an external system — it posts a message.
	r.Post("/api/v1/admin/settings/notifications/{channel}/test", h.Test)
}

func (h *Handler) Test(w http.ResponseWriter, r *http.Request) {
	scope, ok := auth.TenantScopeFromContext(r.Context())
	if !ok {
		admincommon.WriteError(w, http.StatusForbidden, "no active tenant")
		return
	}
	user := auth.UserFromContext(r.Context())
	if user == nil {
		admincommon.WriteError(w, http.StatusForbidden, "no active user")
		return
	}
	if user.Email == "" {
		admincommon.WriteError(w, http.StatusUnprocessableEntity,
			"в вашем профиле нет адреса электронной почты — канал адресует получателей по нему")
		return
	}

	sender, err := h.svc.Sender(r.Context(), scope, chi.URLParam(r, "channel"))
	switch {
	case err == nil:
	case errors.Is(err, notificationchannelsvc.ErrNotConfigured):
		admincommon.WriteError(w, http.StatusConflict,
			"канал ещё не настроен — сохраните настройки и повторите проверку")
		return
	case errors.Is(err, notificationchannelsvc.ErrUnknownChannel),
		errors.Is(err, notificationchannelsvc.ErrNotAvailable):
		admincommon.WriteError(w, http.StatusNotFound, "канал недоступен")
		return
	default:
		admincommon.WriteError(w, http.StatusInternalServerError, "не удалось подготовить канал")
		return
	}

	msg := notifychannel.Message{
		Title: "Проверка канала уведомлений",
		Body:  "Если вы видите это сообщение, канал настроен верно.",
	}
	if err := sender.Send(r.Context(), notifychannel.Target{Email: user.Email}, msg); err != nil {
		// The channel's own message is the only thing that tells the admin whether the
		// token is revoked or the URL is wrong, so it is passed through rather than
		// replaced by a generic failure. It describes the tenant's own configuration
		// and the tenant's own external system — nothing about this server.
		admincommon.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	admincommon.WriteJSON(w, map[string]any{"ok": true})
}
```

- [ ] **Шаг 6: Прогнать тесты хендлеров**

Запустить: `go test ./internal/http/handlers/api/v1/admin/settings/notifications/... -count=1 -v`
Ожидается: PASS. Если `handlertest.User` не проставляет email, тест адресата надо чинить в тесте (собрать контекст с нужным пользователем), а не ослаблять проверку.

- [ ] **Шаг 7: Зарегистрировать маршруты**

В `internal/http/server.go`, в группе `registerAdminRoutes` рядом с `adminaccess`/`adminfeedback`:

```go
		adminnotif.RegisterRoutes(r, adminnotif.New(s.notifChannels))
		adminnotiftest.RegisterRoutes(r, adminnotiftest.New(s.notifChannels))
```
с импортами
```go
	adminnotif "okrs/internal/http/handlers/api/v1/admin/settings/notifications"
	adminnotiftest "okrs/internal/http/handlers/api/v1/admin/settings/notifications/test"
```

- [ ] **Шаг 8: Раздел «Уведомления» в админке пространства**

В `web/static/admin.js` добавить в `ADMIN_SECTIONS` после `health-checkin`:

```js
  {id:'notifications', label:'Уведомления', hint:'Каналы доставки', icon:'🔔'},
```

Компонент (в стиле соседних разделов `admin.js`, на его же хелперах `apiGet`/`apiPut`/`apiPost`/`readErr` и стилях `box`/`btn`/`inp`/`T`):

```jsx
// Каналы уведомлений пространства. Форма каждого канала строится по его же
// дескриптору: этот экран не знает ни одного канала по имени, поэтому канал из
// внешнего репозитория получает рабочую форму без правок здесь.
function NotificationsSection() {
  const [channels,setChannels]=useState([]);
  const [draft,setDraft]=useState({});   // name -> {enabled, values, secret}
  const [msg,setMsg]=useState({});       // name -> текст результата
  const [busy,setBusy]=useState('');

  const load = useCallback(async()=>{
    const r = await apiGet('/api/v1/admin/settings/notifications');
    if (!r) return;
    const list = r.channels||[];
    setChannels(list);
    const d = {};
    list.forEach(c=>{ d[c.name] = {enabled:c.enabled, values:{...(c.values||{})}, secret:''}; });
    setDraft(d);
  },[]);
  useEffect(()=>{ load(); },[load]);

  const setField = (name,key,val)=>setDraft(p=>({...p,[name]:{...p[name],values:{...p[name].values,[key]:val}}}));
  const setFlag  = (name,key,val)=>setDraft(p=>({...p,[name]:{...p[name],[key]:val}}));

  const save = async (c)=>{
    setBusy(c.name); setMsg(m=>({...m,[c.name]:''}));
    const d = draft[c.name];
    const res = await apiPut(`/api/v1/admin/settings/notifications/${encodeURIComponent(c.name)}`,
      {enabled:d.enabled, values:d.values, secret:d.secret});
    setBusy('');
    if (res && res.status===204) { setMsg(m=>({...m,[c.name]:'Сохранено'})); load(); }
    else setMsg(m=>({...m,[c.name]: await readErr(res)}));
  };

  const probe = async (c)=>{
    setBusy(c.name); setMsg(m=>({...m,[c.name]:''}));
    const res = await apiPost(`/api/v1/admin/settings/notifications/${encodeURIComponent(c.name)}/test`, {});
    setBusy('');
    setMsg(m=>({...m,[c.name]: res && res.ok
      ? 'Сообщение отправлено — проверьте мессенджер'
      : await readErr(res)}));
  };

  if (!channels.length) {
    return <div style={box}>
      <h2>Уведомления</h2>
      <div style={{color:T.mutedFg}}>
        Внутренние уведомления (колокольчик) работают всегда и настройки не требуют.
        Внешних каналов у этого пространства нет — их выдаёт администратор системы.
      </div>
    </div>;
  }

  return <div style={box}>
    <h2>Каналы уведомлений</h2>
    <div style={{color:T.mutedFg,marginBottom:12}}>
      Внутренние уведомления работают всегда. Ниже — внешние каналы, выданные пространству.
    </div>
    {channels.map(c=>{
      const d = draft[c.name] || {values:{}};
      const okMsg = msg[c.name]==='Сохранено' || (msg[c.name]||'').startsWith('Сообщение');
      return <div key={c.name} style={{border:'1px solid '+T.cardBorder,borderRadius:10,padding:12,marginBottom:12}}>
        <div style={{display:'flex',alignItems:'center',gap:10,marginBottom:10}}>
          <strong style={{fontSize:15}}>{c.title}</strong>
          <label style={{display:'flex',alignItems:'center',gap:6,color:T.mutedFg}}>
            <input type="checkbox" checked={!!d.enabled} onChange={e=>setFlag(c.name,'enabled',e.target.checked)}/>
            включён
          </label>
        </div>
        {c.fields.map(f=>(
          <div key={f.key} style={{display:'flex',flexDirection:'column',gap:4,marginBottom:10}}>
            <label style={{fontWeight:600}}>{f.label}{f.required && <span style={{color:T.danger}}> *</span>}</label>
            {f.kind==='secret'
              ? <input type="password" autoComplete="new-password" style={inp}
                  placeholder={c.secret_hint ? `сохранено: ${c.secret_hint} — оставьте пустым, чтобы не менять` : 'не задан'}
                  value={d.secret||''} onChange={e=>setFlag(c.name,'secret',e.target.value)}/>
              : <input type={f.kind==='url' ? 'url' : 'text'} style={inp}
                  value={d.values[f.key]||''} onChange={e=>setField(c.name,f.key,e.target.value)}/>}
            {f.hint && <span style={{color:T.dimFg,fontSize:12}}>{f.hint}</span>}
          </div>
        ))}
        <div style={{display:'flex',gap:8,alignItems:'center'}}>
          <button style={btn} disabled={busy===c.name} onClick={()=>save(c)}>Сохранить</button>
          <button style={{...btn,background:T.mutedFg}} disabled={busy===c.name || !c.configured} onClick={()=>probe(c)}>
            Отправить проверку себе
          </button>
          {msg[c.name] && <span style={{color: okMsg ? T.success : T.danger}}>{msg[c.name]}</span>}
        </div>
      </div>;
    })}
  </div>;
}
```

и рендер рядом с остальными разделами:
```jsx
    {section==='notifications' && <NotificationsSection/>}
```

- [ ] **Шаг 9: Проверить, что маршруты попали в защищённую группу**

Гейт по роли живёт в роутере, поэтому его надо увидеть глазами, а не предположить.

Запустить: `rg -n 'adminnotif|RequireTenantAdminMiddleware' internal/http/server.go`
Ожидается: обе регистрации `adminnotif`/`adminnotiftest` находятся **после** строки с `auth.RequireTenantAdminMiddleware` и внутри того же `r.Group(func(r chi.Router) {…})` — то есть в блоке `registerAdminRoutes`, а не в общей tenant-группе.

- [ ] **Шаг 10: Обновить golden и прогнать всё**

Запустить: `go test ./internal/http -run TestRoutesGolden -update-routes && go build ./... && go vet ./... && go test ./... -count=1`
Ожидается: PASS. В golden добавляются три строки: `GET /api/v1/admin/settings/notifications`, `PUT /api/v1/admin/settings/notifications/{channel}`, `POST /api/v1/admin/settings/notifications/{channel}/test`.

- [ ] **Шаг 11: Проверить, что JS парсится**

Запустить: `node -e "const B=require('./web/static/vendor/babel.min.js');const fs=require('fs');['admin.js','system.js'].forEach(f=>B.transform(fs.readFileSync('web/static/'+f,'utf8'),{presets:['react']}));console.log('ok')"`
Ожидается: `ok`.

- [ ] **Шаг 12: Убедиться, что защита от утечки секрета кусается**

Мутация убирает второй барьер в хендлере — тест обязан это заметить.

```bash
mkdir -p /tmp/mut7 && perl -0pe 's/Values: publicValues\(st\),/Values: st.Values,/' \
  internal/http/handlers/api/v1/admin/settings/notifications/handler.go > /tmp/mut7/handler.go
printf '{"Replace":{"%s":"%s"}}' \
  "$PWD/internal/http/handlers/api/v1/admin/settings/notifications/handler.go" "/tmp/mut7/handler.go" > /tmp/mut7/overlay.json
go test -overlay=/tmp/mut7/overlay.json ./internal/http/handlers/api/v1/admin/settings/notifications/ -count=1
```
Ожидается: FAIL в `TestListNeverEchoesPlaintextSecret` (и, возможно, в `TestListDescribesFormAndMasksSecret`). Сборка обязана пройти — падение компиляции ничего не доказывает. Если оба теста остаются зелёными, значит проверка на утечку декоративная и её надо переписать до перехода к задаче 8.

---

## Task 8: Спеки и документация

Код без спек в этом репозитории незавершён: `specs/*` — source of truth, и следующий человек читает их, а не diff.

**Файлы:**
- Изменить: `specs/010-architecture-constraints.md`
- Изменить: `specs/040-api-contract.md`
- Изменить: `specs/050-permissions-and-lifecycle.md`
- Изменить: `specs/070-code-structure.md`
- Изменить: `README-specs.md`
- Изменить: `docs/superpowers/plans/2026-08-27-notifications-tech-debt.md`

**Интерфейсы:** для кода ничего не производит; закрывает §15 дизайн-спеки в части фазы 2a-1.

- [ ] **Шаг 1: `specs/010-architecture-constraints.md`**

В раздел о публичных сеймах (где уже описаны `app` и `web`) добавить третий:

```markdown
### Пакет `notifychannel`

Публичный, а не `internal/`, ровно по той же причине, что `app` и `web`: канал
уведомлений должен собираться во внешнем модуле, а Go запрещает импорт `internal/`
за пределами `okrs/`. Пакет содержит только контракт (`Channel`, `Sender`,
`Descriptor`, `Target`, `Message`, `Field`) и не зависит ни от чего, кроме
стандартной библиотеки; это проверяется в тесте через `go list -deps ./notifychannel/`.

Каналы подключаются значением через `app.Config.NotificationChannels`, а не
регистрацией в `init()`: список каналов сборки должен быть виден в одном месте
рядом с `main`, а не собираться из побочных эффектов импортов.

Секреты каналов шифруются AES-256-GCM (`internal/platform/secretbox`) ключом из
`NOTIFICATIONS_SECRET_KEY` (base64, 32 байта). Ключ не задан — каналы с секретом
настроить нельзя, остальное приложение работает без изменений.
```

Если в спеке есть перечень переменных окружения, добавить туда `NOTIFICATIONS_SECRET_KEY` с этим же описанием.

- [ ] **Шаг 2: `specs/040-api-contract.md`**

В таблицу маршрутов добавить четыре строки — в тех же формулировках, что попали в `routes.golden`:

| Метод | Путь | Доступ | Назначение |
|---|---|---|---|
| GET | `/api/v1/system/notification-channels` | system admin | каналы, собранные в бинарь |
| GET | `/api/v1/admin/settings/notifications` | admin пространства | доступные каналы и их настройки |
| PUT | `/api/v1/admin/settings/notifications/{channel}` | admin пространства | сохранить настройку канала |
| POST | `/api/v1/admin/settings/notifications/{channel}/test` | admin пространства | отправить проверочное сообщение себе |

Отдельным абзацем зафиксировать два контрактных решения, которые иначе выглядят произволом:

```markdown
Канал, недоступный пространству, и канал, которого нет в сборке, отвечают
одинаково — `404`. Разные ответы позволили бы администратору пространства
перебором получить полный список каналов продукта, включая невыданные.

Секрет канала наружу не возвращается никогда. В ответах есть только `secret_hint` —
маска вида `••••4821`. Пустой `secret` в `PUT` означает «не менять сохранённый»,
а не «стереть».
```

- [ ] **Шаг 3: `specs/050-permissions-and-lifecycle.md`**

```markdown
Настройки каналов уведомлений читает и меняет только администратор пространства:
там хранятся секреты подключения. Участник пространства не видит даже маску.
Права проверяет `auth.RequireTenantAdminMiddleware` на группе маршрутов.

Какие каналы доступны пространству, решает пользователь системного уровня —
через entitlement `entitlement.notifications.<канал>`. Администратор пространства
включает, выключает и настраивает только выданные каналы и не видит остальных.
```

- [ ] **Шаг 4: `specs/070-code-structure.md`**

Добавить в карту пакетов:

```markdown
- `notifychannel/` — публичный контракт канала уведомлений (только stdlib)
- `notifychannel/mattermost/` — канал Mattermost; образец для внешних реализаций
- `internal/platform/secretbox/` — AES-256-GCM для секретов каналов
- `internal/store/notificationchannels/` — конфигурация каналов и привязки аккаунтов
- `internal/service/notificationchannel/` — конфиг, шифрование, гейт по entitlements, резолв `Sender`
- `internal/http/handlers/api/v1/system/notificationchannels/` — список каналов сборки
- `internal/http/handlers/api/v1/admin/settings/notifications/` — настройки каналов пространства
- `internal/http/handlers/api/v1/admin/settings/notifications/test/` — проверочная отправка
```

- [ ] **Шаг 5: `README-specs.md`**

Одна строка в оглавление: каналы уведомлений расширяемы через публичный пакет
`notifychannel` и подключаются значением рядом с `main`.

- [ ] **Шаг 6: Обновить реестр техдолга**

В `docs/superpowers/plans/2026-08-27-notifications-tech-debt.md`:
- закрыть строки, которые фаза 2a-1 действительно закрыла (публичный сейм каналов);
- добавить то, что она осознанно не сделала:
  - ротация `NOTIFICATIONS_SECRET_KEY` (перешифровка сохранённых секретов) не реализована — смена ключа сегодня ломает все настроенные каналы;
  - `notification_identities` заведена, но заполняется только каналами с `Linker`; в 2a-1 таких каналов нет;
  - привязки каналов к тарифам нет по решению пользователя — гейт сейчас только entitlement;
  - у канала нет ограничения частоты обращений к внешнему API; в 2a-1 это неважно (шлёт только кнопка «Проверить»), но воркер доставки в 2a-2 обязан это учесть;
  - `POST .../test` не ограничен по частоте: администратор может дёргать внешний API кнопкой. Отдельный риск невелик (нужны права админа), но в 2a-2 разумно закрыть общим лимитером.

- [ ] **Шаг 7: Проверить, что спеки и роутер сходятся**

Запустить: `go test ./internal/http -run 'TestSpecRouteTableMatchesRouter|TestRoutesGolden' -count=1 -v`
Ожидается: PASS. Тест сверяет таблицу спеки с реальным роутером; если он падает, расходится либо путь, либо метод — и правильный ответ почти всегда «поправить спеку», а не «ослабить тест».

- [ ] **Шаг 8: Финальная проверка**

Запустить:
```bash
go build ./... && go vet ./... && go test ./... -count=1 && \
gofmt -l notifychannel internal/platform/secretbox internal/service/notificationchannel \
  internal/store/notificationchannels \
  internal/http/handlers/api/v1/system/notificationchannels \
  internal/http/handlers/api/v1/admin/settings/notifications
```
Ожидается: всё зелено, `gofmt` молчит. `gofmt -l internal/` целиком не запускать: рабочее дерево в CRLF, и он отметит ~450 файлов, не тронутых этой работой.

Коммиты не делать — пользователь коммитит сам (CLAUDE.md, правило 8).
