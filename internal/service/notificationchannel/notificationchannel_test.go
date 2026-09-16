package notificationchannel_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/platform/entitlements"
	"okrs/internal/platform/secretbox"
	notificationchannelsvc "okrs/internal/service/notificationchannel"
	"okrs/internal/store/notificationchannels"
	"okrs/notifychannel"
)

// fakeRepo — стор в памяти: сервис не должен требовать БД для своей логики.
// Под мьютексом, потому что сервис законно ходит сюда из нескольких горутин:
// доставка резолвит отправителей параллельно, а конструирование канала
// сериализовано по ключу, но не глобально. Фейк, не выдерживающий того, что
// выдерживает настоящий стор, ловил бы не дефекты, а сам себя.
type fakeRepo struct {
	mu   sync.Mutex
	rows map[string]notificationchannels.Config
}

func (f *fakeRepo) List(context.Context, domain.TenantScope) ([]notificationchannels.Config, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]notificationchannels.Config, 0, len(f.rows))
	for _, c := range f.rows {
		out = append(out, c)
	}
	return out, nil
}

func (f *fakeRepo) Get(_ context.Context, _ domain.TenantScope, ch string) (notificationchannels.Config, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.rows[ch]
	return c, ok, nil
}

func (f *fakeRepo) Upsert(_ context.Context, _ domain.TenantScope, c notificationchannels.Config, _ int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rows == nil {
		f.rows = map[string]notificationchannels.Config{}
	}
	f.rows[c.Channel] = c
	return nil
}

// bump подменяет хранимую строку так, как это сделал бы Save с другой реплики:
// содержимое и время изменения новые, через реестр никто не проходил.
func (f *fakeRepo) bump(channel string, values map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row := f.rows[channel]
	row.Values = values
	row.UpdatedAt = time.Now().Add(time.Second)
	f.rows[channel] = row
}

// gate — управляемая реализация entitlements: разрешает только перечисленные ключи.
// Отвечает на вопрос «тариф не запрещает», а не «канал выдан» — это две разные
// вещи после починки, см. fakeGrants ниже.
type gate struct{ allow map[string]bool }

func (g gate) Has(_ domain.TenantScope, key string) bool { return g.allow[key] }
func (g gate) Limit(domain.TenantScope, string) int64    { return -1 }

// fakeGrants — явные выдачи каналов тенанту: то, что в проде системный
// администратор пишет руками через `/system` и что оседает в tenant_settings под
// ключом `entitlement.notifications.<name>`. TenantEntitlements в проде отдаёт
// эти ключи уже без префикса `entitlement.` — fakeGrants воспроизводит ровно это,
// а не оборачивает settings.Service, поэтому сервис тестируется без БД.
//
// granted[name] == false моделирует «тумблер выключили» (ключ есть, значение
// false) — отдельно от отсутствия ключа в granted вовсе («тумблер никогда не
// трогали»). Оба случая обязаны вести к «недоступен», но по разным причинам, и
// тест TestExplicitFalseGrantIsNotTreatedAsGranted проверяет именно первый.
//
// raw — тот же ключ, но с сырым json.RawMessage вместо bool: даёт тестам
// подставить значение, которое не разбирается как булево вовсе (строка, число,
// битый JSON) — granted одним лишь bool такое не выразить, а именно это
// значение реально пишется в tenant_settings и обязано читаться безопасно.
// Если имя есть и там, и там, побеждает raw.
type fakeGrants struct {
	granted map[string]bool
	raw     map[string]json.RawMessage
}

func (g fakeGrants) TenantEntitlements(context.Context, domain.TenantScope) (map[string]json.RawMessage, error) {
	out := make(map[string]json.RawMessage, len(g.granted)+len(g.raw))
	for name, on := range g.granted {
		v, err := json.Marshal(on)
		if err != nil {
			panic(err) // bool всегда маршалится
		}
		out["notifications."+name] = v
	}
	for name, v := range g.raw {
		out["notifications."+name] = v
	}
	return out, nil
}

// grantedOnly — сокращение для самого частого случая: канал "fake" выдан явно,
// без посторонних ключей.
func grantedOnly(names ...string) fakeGrants {
	g := fakeGrants{granted: map[string]bool{}}
	for _, n := range names {
		g.granted[n] = true
	}
	return g
}

type recordingSender struct {
	built    notifychannel.Settings
	deps     notifychannel.Deps
	flushed  int
	sentNow  int
	accepted int
	// flushGate задерживает выгрузку, flushEntered сообщает, что она началась.
	// Вопрос «какие замки удерживаются во время сетевой операции» наблюдается
	// только так: надо дождаться, что выгрузка реально идёт, и лишь потом
	// проверять, проходят ли другие вызовы.
	flushGate    chan struct{}
	flushEntered chan struct{}
}

func (r *recordingSender) Send(context.Context, notifychannel.Target, notifychannel.Message) error {
	r.accepted++
	return nil
}

func (r *recordingSender) SendNow(context.Context, notifychannel.Target, notifychannel.Message) error {
	r.sentNow++
	return nil
}

func (r *recordingSender) Flush(context.Context) error {
	r.flushed++
	if r.flushEntered != nil {
		select {
		case r.flushEntered <- struct{}{}:
		default:
		}
	}
	if r.flushGate != nil {
		<-r.flushGate
	}
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
		New: func(d notifychannel.Deps) (notifychannel.Sender, error) {
			if d.Settings.Secret == "" {
				return nil, notifychannel.ErrMissingSecret
			}
			rs := &recordingSender{built: d.Settings, deps: d}
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

// newSvc собирает сервис с полностью проходящим гейтом для канала "fake": и
// entitlements (allow), и явная выдача (granted). Тесты, которым нужен гейт,
// перекрывающий одно из двух условий, собирают сервис через
// notificationchannelsvc.New напрямую, а не через этот хелпер.
func newSvc(t *testing.T, allow map[string]bool, granted fakeGrants, built **recordingSender) (*notificationchannelsvc.Service, *fakeRepo) {
	t.Helper()
	repo := &fakeRepo{rows: map[string]notificationchannels.Config{}}
	svc, err := notificationchannelsvc.New(repo, newKey(t),
		[]notifychannel.Channel{testChannel(built)}, gate{allow: allow}, granted, nil)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	return svc, repo
}

var scope = domain.TenantScope{TenantID: 1}

// entitledAndGranted — bare-ключ гейта для канала "fake" в обоих условиях сразу:
// то, что нужно большинству тестов, которые проверяют не гейт, а поведение
// вокруг него (шифрование, обязательные поля и т.д.).
var entitledAndGranted = map[string]bool{"entitlement.notifications.fake": true}

// Канал ВЫДАН тенанту явно, но тариф запрещает — недоступен. Это второе условие
// гейта: явная выдача не обходит тарифный запрет. Раньше единственной проверкой
// в Save/Available была entitlements.Has, и этот тест проверял её в одиночку;
// теперь он проверяет её как одно из двух необходимых условий, при явно
// выполненном первом — так что ослабления нет, изменился только состав входа.
func TestGrantedChannelStillBlockedWhenTariffForbids(t *testing.T) {
	var built *recordingSender
	svc, repo := newSvc(t, map[string]bool{}, grantedOnly("fake"), &built)

	got, err := svc.Available(context.Background(), scope)
	if err != nil {
		t.Fatalf("available: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("тариф запрещает канал, а он попал в доступные: %+v", got)
	}
	err = svc.Save(context.Background(), scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: true, Values: map[string]any{"base_url": "https://x"}, Secret: "s",
	}, 1)
	if !errors.Is(err, notificationchannelsvc.ErrNotAvailable) {
		t.Fatalf("got %v, want ErrNotAvailable", err)
	}
	if len(repo.rows) != 0 {
		t.Fatal("запись произошла, несмотря на отказ")
	}
}

// Канал НЕ выдан тенанту явно, хотя тариф разрешает всё — недоступен. Это и есть
// починка коробочного дефекта: сборка использует entitlements.UnlimitedEntitlements
// (тот же тип, что реально подставляет app.go, когда EntitlementsName не задан) —
// реализацию, которая раньше в одиночку решала доступность и отвечала true на
// любой ключ. До починки этот тест был бы TestUnentitledChannelIsInvisibleAndUnwritable
// наоборот: тариф разрешал бы всё, и канал был бы виден без какой-либо явной
// выдачи — то есть ровно дефект, который увидел пользователь на /admin.
func TestChannelUnavailableWithoutExplicitGrantEvenWhenTariffAllowsEverything(t *testing.T) {
	var built *recordingSender
	repo := &fakeRepo{rows: map[string]notificationchannels.Config{}}
	svc, err := notificationchannelsvc.New(repo, newKey(t),
		[]notifychannel.Channel{testChannel(&built)},
		entitlements.UnlimitedEntitlements{}, // коробочный тариф: разрешает всё
		grantedOnly(),                        // но системный администратор ничего не выдавал
		nil,
	)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	got, availErr := svc.Available(context.Background(), scope)
	if availErr != nil {
		t.Fatalf("available: %v", availErr)
	}
	if len(got) != 0 {
		t.Fatalf("невыданный канал попал в доступные при разрешающем тарифе: %+v", got)
	}
	ok, isAvailErr := svc.IsAvailable(context.Background(), scope, "fake")
	if isAvailErr != nil {
		t.Fatalf("is available: %v", isAvailErr)
	}
	if ok {
		t.Fatal("IsAvailable обязан быть false без явной выдачи, даже когда тариф разрешает всё")
	}
	saveErr := svc.Save(context.Background(), scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: true, Values: map[string]any{"base_url": "https://x"}, Secret: "s",
	}, 1)
	if !errors.Is(saveErr, notificationchannelsvc.ErrNotAvailable) {
		t.Fatalf("got %v, want ErrNotAvailable", saveErr)
	}
}

// Канал выдан явно И тариф разрешает — доступен. Симметричная пара к тесту выше:
// подтверждает, что починка не переусердствовала и не заблокировала легитимный
// случай (тот самый, который увидит коробочный админ после починки: он выдал
// канал через /system, и он появляется на /admin).
func TestChannelAvailableWhenGrantedAndTariffAllows(t *testing.T) {
	var built *recordingSender
	repo := &fakeRepo{rows: map[string]notificationchannels.Config{}}
	svc, err := notificationchannelsvc.New(repo, newKey(t),
		[]notifychannel.Channel{testChannel(&built)},
		entitlements.UnlimitedEntitlements{},
		grantedOnly("fake"),
		nil,
	)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	got, availErr := svc.Available(context.Background(), scope)
	if availErr != nil {
		t.Fatalf("available: %v", availErr)
	}
	if len(got) != 1 || got[0].Name != "fake" {
		t.Fatalf("выданный канал обязан быть доступен: %+v", got)
	}
	ok, isAvailErr := svc.IsAvailable(context.Background(), scope, "fake")
	if isAvailErr != nil || !ok {
		t.Fatalf("IsAvailable: got (%v, %v), want (true, nil)", ok, isAvailErr)
	}
}

// Значение выдачи false — тумблер когда-то включили и потом выключили — обязано
// вести к «недоступен», а не «ключ есть, значит выдан». Отличается от
// TestChannelUnavailableWithoutExplicitGrantEvenWhenTariffAllowsEverything тем,
// что там ключа нет вовсе, а здесь он есть со значением false: если бы
// grantedChannels проверяла только наличие ключа, этот тест бы упал, а тот —
// остался зелёным.
func TestExplicitFalseGrantIsNotTreatedAsGranted(t *testing.T) {
	var built *recordingSender
	svc, _ := newSvc(t, entitledAndGranted, fakeGrants{granted: map[string]bool{"fake": false}}, &built)

	got, err := svc.Available(context.Background(), scope)
	if err != nil {
		t.Fatalf("available: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("выключенная выдача (false) не должна делать канал доступным: %+v", got)
	}
}

// Значение выдачи, которое не разбирается как bool вовсе (опечатка при ручной
// правке tenant_settings, число, строка "true" вместо true, битый JSON) —
// тот же класс защиты, что и explicit false выше: тумблер правит человек, и
// опечатка не должна молча открыть канал. grantedChannels обязана расценивать
// ошибку разбора как «не выдано», а не пропускать её мимо проверки.
func TestUnparseableGrantValueIsNotTreatedAsGranted(t *testing.T) {
	var built *recordingSender
	cases := map[string]json.RawMessage{
		"строка вместо bool": json.RawMessage(`"true"`),
		"число":              json.RawMessage(`1`),
		"битый JSON":         json.RawMessage(`not-json`),
		"null":               json.RawMessage(`null`),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			svc, _ := newSvc(t, entitledAndGranted,
				fakeGrants{raw: map[string]json.RawMessage{"fake": raw}}, &built)
			got, err := svc.Available(context.Background(), scope)
			if err != nil {
				t.Fatalf("available: %v", err)
			}
			if len(got) != 0 {
				t.Fatalf("нераспознанное значение выдачи не должно делать канал доступным: %+v", got)
			}
		})
	}
}

// Descriptors() продолжает показывать ВСЁ, что собрано в бинарь, независимо от
// гейта: системной панели нужен полный список, чтобы было что разрешать.
// Available() фильтрует по обоим условиям гейта — здесь оба ложны.
func TestDescriptorsShowsBuildWhileAvailableShowsGranted(t *testing.T) {
	var built *recordingSender
	svc, _ := newSvc(t, map[string]bool{}, grantedOnly(), &built)

	if len(svc.Descriptors()) != 1 {
		t.Fatal("Descriptors обязан показывать канал сборки независимо от гейта")
	}
	got, err := svc.Available(context.Background(), scope)
	if err != nil {
		t.Fatalf("available: %v", err)
	}
	if len(got) != 0 {
		t.Fatal("Available обязан фильтровать и по выдаче, и по entitlements")
	}
}

// Секрет уходит в БД зашифрованным и никогда не возвращается наружу открытым.
//
// Ключевая проверка sanitize: вход НАРОЧНО кладёт значение под именем секретного
// поля (SecretField == "token") в SaveInput.Values, отдельно от Secret. Если бы
// sanitize была тождественной функцией (return values), это значение осело бы и
// в repo.rows["fake"].Values, и в states[0].Values — оба конца и проверяются.
func TestSaveEncryptsSecretAndExposesOnlyHint(t *testing.T) {
	var built *recordingSender
	svc, repo := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)

	const secret = "token-abcdef4821"
	const leaked = "утечка-должна-быть-вырезана"
	if err := svc.Save(context.Background(), scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: true,
		Values: map[string]any{"base_url": "https://x", "token": leaked}, Secret: secret,
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
	if _, present := row.Values["token"]; present {
		t.Fatalf("секретное поле осталось в сохранённых Values: %+v", row.Values)
	}

	states, err := svc.List(context.Background(), scope)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(states) != 1 || states[0].SecretHint != "••••4821" {
		t.Fatalf("состояние: %+v", states)
	}
	// В состоянии наружу не должно быть ни одного поля с плейнтекстом.
	if _, present := states[0].Values["token"]; present {
		t.Fatalf("секретное поле просочилось в Values: %+v", states[0].Values)
	}
}

// Пустой секрет при сохранении означает «не менять», а не «стереть»: форма в
// админке показывает маску, и пользователь, правя только base_url, не должен
// потерять токен.
func TestEmptySecretKeepsPrevious(t *testing.T) {
	var built *recordingSender
	svc, repo := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)
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
	svc, _ := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)
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
	svc, _ := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)

	_, err := svc.Sender(context.Background(), scope, "fake")
	if !errors.Is(err, notificationchannelsvc.ErrNotConfigured) {
		t.Fatalf("got %v, want ErrNotConfigured", err)
	}
}

// Sender проверяет гейт, а не только Save: канал, у которого тенант когда-то был
// выдан и настроен, а затем entitlements отозвали (тариф понижен), не должен
// продолжать резолвиться в рабочий Sender. Это самый дорогой путь гейта — им
// пользуется воркер доставки фазы 2a-2. Явная выдача остаётся истинной в обоих
// сервисах — отзывается именно тарифное условие, изолированно от выдачи.
func TestSenderRejectsWhenEntitlementRevoked(t *testing.T) {
	var built *recordingSender
	repo := &fakeRepo{rows: map[string]notificationchannels.Config{}}
	key := newKey(t)

	grantedSvc, err := notificationchannelsvc.New(repo, key,
		[]notifychannel.Channel{testChannel(&built)},
		gate{allow: entitledAndGranted}, grantedOnly("fake"), nil)
	if err != nil {
		t.Fatalf("new (granted): %v", err)
	}
	if err := grantedSvc.Save(context.Background(), scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: true,
		Values: map[string]any{"base_url": "https://x"}, Secret: "секрет",
	}, 1); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Тот же repo, тот же ключ, та же выдача, но entitlement больше не разрешён —
	// как будто тариф пространства понизили после того, как тенант настроил канал.
	revokedSvc, err := notificationchannelsvc.New(repo, key,
		[]notifychannel.Channel{testChannel(&built)}, gate{allow: map[string]bool{}}, grantedOnly("fake"), nil)
	if err != nil {
		t.Fatalf("new (revoked): %v", err)
	}
	if _, err := revokedSvc.Sender(context.Background(), scope, "fake"); !errors.Is(err, notificationchannelsvc.ErrNotAvailable) {
		t.Fatalf("got %v, want ErrNotAvailable", err)
	}
}

// Sender таким же образом проверяет и отзыв ВЫДАЧИ — симметрично предыдущему
// тесту, но с обратным условием: тариф остаётся разрешающим, а выдачу снимают
// (системный администратор выключил тумблер после того, как тенант настроил
// канал). До починки этот путь не проверялся вовсе — Sender смотрел только на
// entitlements.
func TestSenderRejectsWhenGrantRevoked(t *testing.T) {
	var built *recordingSender
	repo := &fakeRepo{rows: map[string]notificationchannels.Config{}}
	key := newKey(t)

	grantedSvc, err := notificationchannelsvc.New(repo, key,
		[]notifychannel.Channel{testChannel(&built)},
		gate{allow: entitledAndGranted}, grantedOnly("fake"), nil)
	if err != nil {
		t.Fatalf("new (granted): %v", err)
	}
	if err := grantedSvc.Save(context.Background(), scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: true,
		Values: map[string]any{"base_url": "https://x"}, Secret: "секрет",
	}, 1); err != nil {
		t.Fatalf("save: %v", err)
	}

	revokedSvc, err := notificationchannelsvc.New(repo, key,
		[]notifychannel.Channel{testChannel(&built)}, gate{allow: entitledAndGranted}, grantedOnly(), nil)
	if err != nil {
		t.Fatalf("new (grant revoked): %v", err)
	}
	if _, err := revokedSvc.Sender(context.Background(), scope, "fake"); !errors.Is(err, notificationchannelsvc.ErrNotAvailable) {
		t.Fatalf("got %v, want ErrNotAvailable", err)
	}
}

// IsAvailable — публичная проверка гейта: истинна только для выданного и
// разрешённого тарифом канала, ложна для канала, которого нет в сборке, и ложна
// для канала, который выдан, но запрещён тарифом. Третий случай — отдельная
// проверка именно для IsAvailable, а не переиспользование Available/Save: у
// IsAvailable сегодня нет живого потребителя внутри сервиса (Available и Save
// реализуют условие каждый своим кодом), но метод поимённо назван одним из
// путей гейта в specs/050 и предназначен воркеру доставки фазы 2a-2 — мутация,
// заставляющая IsAvailable смотреть только на выдачу и игнорировать тариф, не
// задевает ни Available, ни Save и без этого случая прошла бы весь пакет
// незамеченной.
func TestIsAvailable(t *testing.T) {
	var built *recordingSender
	svc, _ := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)

	ok, err := svc.IsAvailable(context.Background(), scope, "fake")
	if err != nil || !ok {
		t.Fatalf("выданный и разрешённый канал должен быть доступен: (%v, %v)", ok, err)
	}
	ok, err = svc.IsAvailable(context.Background(), scope, "other-tenant-scope-does-not-matter-here")
	if err != nil || ok {
		t.Fatalf("несуществующий канал не должен быть доступен: (%v, %v)", ok, err)
	}

	forbiddenSvc, _ := newSvc(t, map[string]bool{}, grantedOnly("fake"), &built)
	ok, err = forbiddenSvc.IsAvailable(context.Background(), scope, "fake")
	if err != nil || ok {
		t.Fatalf("выданный, но запрещённый тарифом канал не должен быть доступен: (%v, %v)", ok, err)
	}
}

// Неизвестное имя канала отвергается до любой работы с БД или гейтом.
func TestUnknownChannelRejected(t *testing.T) {
	var built *recordingSender
	svc, _ := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)

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
		gate{allow: entitledAndGranted}, grantedOnly("fake"), nil)
	if err != nil {
		t.Fatalf("сервис обязан собираться без ключа: %v", err)
	}
	saveErr := svc.Save(context.Background(), scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: true, Secret: "s",
		// base_url заполнен: форма валидна, и единственная причина отказа —
		// отсутствие ключа шифрования, а не незаполненное обязательное поле.
		Values: map[string]any{"base_url": "https://x"},
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
		gate{}, grantedOnly(), nil)
	if err == nil {
		t.Fatal("два канала с одинаковым Descriptor.Name должны отвергаться")
	}
}

// Канал без конструктора — ошибка сборки, а не паника при первом Sender().
func TestNewRejectsChannelWithoutConstructor(t *testing.T) {
	broken := notifychannel.Channel{
		Descriptor: notifychannel.Descriptor{Name: "broken", Title: "Сломанный"},
		New:        nil,
	}
	_, err := notificationchannelsvc.New(&fakeRepo{}, newKey(t),
		[]notifychannel.Channel{broken}, gate{}, grantedOnly(), nil)
	if err == nil {
		t.Fatal("канал с nil-конструктором должен отвергаться на сборке")
	}
}

// Сборка без ChannelGrants — ошибка сборки: без него гейт не может ответить на
// вопрос «выдан ли канал» и упал бы позже в рантайме на nil-указателе. Дешевле
// отказать на сборке, как и с nil-конструктором канала выше.
func TestNewRejectsNilGrants(t *testing.T) {
	var built *recordingSender
	_, err := notificationchannelsvc.New(&fakeRepo{}, newKey(t),
		[]notifychannel.Channel{testChannel(&built)}, gate{}, nil, nil)
	if err == nil {
		t.Fatal("сборка без ChannelGrants должна отвергаться")
	}
}

// Включить канал с секретным полем, не дав ни нового секрета, ни имея старого,
// значит записать в БД «включённый», но неработоспособный канал — узнает об
// этом только воркер доставки в рантайме. Save обязан отказать сразу.
func TestSaveRejectsEnablingWithoutSecret(t *testing.T) {
	var built *recordingSender
	svc, repo := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)

	err := svc.Save(context.Background(), scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: true, Values: map[string]any{"base_url": "https://x"},
	}, 1)
	if !errors.Is(err, notificationchannelsvc.ErrSecretRequired) {
		t.Fatalf("got %v, want ErrSecretRequired", err)
	}
	if len(repo.rows) != 0 {
		t.Fatal("запись произошла, несмотря на отказ")
	}
}

// Сохранить канал ВЫКЛЮЧЕННЫМ и без секрета — законно: он просто не работает,
// но и не притворяется работающим.
func TestSaveAllowsDisablingWithoutSecret(t *testing.T) {
	var built *recordingSender
	svc, repo := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)

	err := svc.Save(context.Background(), scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: false, Values: map[string]any{"base_url": "https://x"},
	}, 1)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	row, ok := repo.rows["fake"]
	if !ok {
		t.Fatal("запись не произошла")
	}
	if row.Enabled {
		t.Fatal("канал должен остаться выключенным")
	}
}

// Двухшаговый обход ErrSecretRequired: сохранить канал выключенным без
// секрета (законно), а затем попытаться включить его — тоже без секрета.
// hadPrev на втором шаге истинно, но у сохранённой строки нет настоящего
// секрета (SecretEnc пуст), поэтому "оставить прежний секрет" не должно
// срабатывать — иначе в базе оказался бы включённый канал вовсе без секрета.
func TestSaveRejectsEnablingAfterDisabledWithoutSecret(t *testing.T) {
	var built *recordingSender
	svc, repo := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)
	ctx := context.Background()

	if err := svc.Save(ctx, scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: false, Values: map[string]any{"base_url": "https://x"},
	}, 1); err != nil {
		t.Fatalf("save 1 (disabled, no secret): %v", err)
	}

	err := svc.Save(ctx, scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: true, Values: map[string]any{"base_url": "https://x"},
	}, 1)
	if !errors.Is(err, notificationchannelsvc.ErrSecretRequired) {
		t.Fatalf("got %v, want ErrSecretRequired", err)
	}
	row := repo.rows["fake"]
	if row.Enabled {
		t.Fatal("канал не должен был стать включённым без секрета")
	}
	if len(row.SecretEnc) != 0 {
		t.Fatalf("у канала не должно быть секрета: %v", row.SecretEnc)
	}
}

// failingSender всегда возвращает заданную ошибку — управляемая замена реальному
// каналу для проверки того, что Sender() делает с текстом ошибки Send.
type failingSender struct{ err error }

func (f failingSender) Send(context.Context, notifychannel.Target, notifychannel.Message) error {
	return f.err
}

func (f failingSender) SendNow(context.Context, notifychannel.Target, notifychannel.Message) error {
	return f.err
}

func (f failingSender) Flush(context.Context) error { return f.err }

func failingChannel(name string, secretField string, err error) notifychannel.Channel {
	d := notifychannel.Descriptor{Name: name, Title: "Фейковый (падающий)", SecretField: secretField}
	if secretField != "" {
		d.Fields = []notifychannel.Field{
			{Key: "base_url", Label: "URL", Required: true, Kind: notifychannel.FieldURL},
			{Key: secretField, Label: "Секрет", Required: true, Kind: notifychannel.FieldSecret},
		}
	}
	return notifychannel.Channel{
		Descriptor: d,
		New: func(notifychannel.Deps) (notifychannel.Sender, error) {
			return failingSender{err: err}, nil
		},
	}
}

// errUpstream — сентинел, обёрнутый внутрь ошибки доставки: маскировка обязана
// сохранить его достижимым через errors.Is/errors.As (ровно так работает
// mattermost.IsPermanent), а не заменить ошибку целиком непрозрачной строкой.
var errUpstream = errors.New("failing sender: upstream")

// Секрет, попавший в текст ошибки доставки (как токен Telegram в URL запроса),
// не должен долетать до администратора: правило проекта «плейнтекст секрета
// никогда не покидает сервер» абсолютно, а не «пока используемые каналы аккуратны».
func TestSenderMasksSecretInDeliveryError(t *testing.T) {
	const secret = "секрет-в-url-4821"
	upstream := fmt.Errorf("telegram: request to https://api.telegram.org/bot%s/sendMessage failed: %w", secret, errUpstream)

	repo := &fakeRepo{rows: map[string]notificationchannels.Config{}}
	svc, err := notificationchannelsvc.New(repo, newKey(t),
		[]notifychannel.Channel{failingChannel("failing", "token", upstream)},
		gate{allow: map[string]bool{"entitlement.notifications.failing": true}}, grantedOnly("failing"), nil)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx := context.Background()
	if err := svc.Save(ctx, scope, notificationchannelsvc.SaveInput{
		Channel: "failing", Enabled: true, Values: map[string]any{"base_url": "https://x"}, Secret: secret,
	}, 1); err != nil {
		t.Fatalf("save: %v", err)
	}

	sender, err := svc.Sender(ctx, scope, "failing")
	if err != nil {
		t.Fatalf("sender: %v", err)
	}
	sendErr := sender.Send(ctx, notifychannel.Target{Email: "a@b.c"}, notifychannel.Message{Title: "t"})
	if sendErr == nil {
		t.Fatal("ожидалась ошибка доставки")
	}
	if strings.Contains(sendErr.Error(), secret) {
		t.Fatalf("секрет утёк в тексте ошибки: %s", sendErr.Error())
	}
	if !errors.Is(sendErr, errUpstream) {
		t.Fatal("маскировка обязана сохранить цепочку errors.Is до исходной ошибки канала")
	}
}

// Канал без секрета не оборачивается вовсе: маскировать нечего, и платить за
// обёртку там, где она ничего не защищает, незачем. Проверяется через identity
// возвращённой ошибки — обёртка заменяет значение ошибки, «пропуск» его сохраняет.
func TestSenderDoesNotWrapWhenChannelHasNoSecret(t *testing.T) {
	errNoSecret := errors.New("boom, no secret involved")
	repo := &fakeRepo{rows: map[string]notificationchannels.Config{}}
	svc, err := notificationchannelsvc.New(repo, newKey(t),
		[]notifychannel.Channel{failingChannel("open", "", errNoSecret)},
		gate{allow: map[string]bool{"entitlement.notifications.open": true}}, grantedOnly("open"), nil)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx := context.Background()
	if err := svc.Save(ctx, scope, notificationchannelsvc.SaveInput{
		Channel: "open", Enabled: true, Values: map[string]any{},
	}, 1); err != nil {
		t.Fatalf("save: %v", err)
	}

	sender, err := svc.Sender(ctx, scope, "open")
	if err != nil {
		t.Fatalf("sender: %v", err)
	}
	sendErr := sender.Send(ctx, notifychannel.Target{Email: "a@b.c"}, notifychannel.Message{Title: "t"})
	if sendErr != errNoSecret {
		t.Fatalf("канал без секрета не должен оборачиваться: got %v, want ровно errNoSecret", sendErr)
	}
}

// Успешная отправка через канал с секретом остаётся успешной: обёртка не должна
// превращать nil в ошибку и не должна что-либо аллоцировать на этом пути.
func TestSenderWithSecretStaysNilOnSuccess(t *testing.T) {
	var built *recordingSender
	svc, _ := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)
	ctx := context.Background()
	if err := svc.Save(ctx, scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: true, Values: map[string]any{"base_url": "https://x"}, Secret: "секрет-99",
	}, 1); err != nil {
		t.Fatalf("save: %v", err)
	}

	sender, err := svc.Sender(ctx, scope, "fake")
	if err != nil {
		t.Fatalf("sender: %v", err)
	}
	if err := sender.Send(ctx, notifychannel.Target{Email: "a@b.c"}, notifychannel.Message{Title: "t"}); err != nil {
		t.Fatalf("успешная отправка должна остаться успешной сквозь обёртку маскировки: %v", err)
	}
}

// Обязательные поля проверяются ровно по той же причине, что и секрет
// (ErrSecretRequired): включённый канал с пустым base_url ложится в БД как
// «работает», кнопка «Проверить» падает, а воркер фазы 2a-2 будет ронять на нём
// каждую доставку. Отказ обязан называть поле — форма генерится из дескриптора,
// и только сервер знает, какой инпут остался пустым.
func TestEnablingWithEmptyRequiredFieldIsRejected(t *testing.T) {
	var built *recordingSender
	svc, repo := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)

	cases := map[string]map[string]any{
		"поля нет вовсе":   {},
		"поле пустое":      {"base_url": ""},
		"поле из пробелов": {"base_url": "   "},
		"поле равно null":  {"base_url": nil},
	}
	for name, values := range cases {
		t.Run(name, func(t *testing.T) {
			err := svc.Save(context.Background(), scope, notificationchannelsvc.SaveInput{
				Channel: "fake", Enabled: true, Values: values, Secret: "s",
			}, 1)
			if !errors.Is(err, notificationchannelsvc.ErrFieldRequired) {
				t.Fatalf("got %v, want ErrFieldRequired", err)
			}
			var fe *notificationchannelsvc.FieldRequiredError
			if !errors.As(err, &fe) || fe.Key != "base_url" || fe.Label != "URL" {
				t.Fatalf("ошибка не называет поле: %+v", fe)
			}
			if _, stored := repo.rows["fake"]; stored {
				t.Fatal("неработоспособный канал не должен попадать в БД")
			}
		})
	}
}

// Выключенный канал сохраняется с любыми пустыми полями: это черновик настроек,
// а не обещание, что он работает. Симметрично правилу для секрета.
func TestDisabledChannelMayBeSavedIncomplete(t *testing.T) {
	var built *recordingSender
	svc, repo := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)

	err := svc.Save(context.Background(), scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: false, Values: map[string]any{},
	}, 1)
	if err != nil {
		t.Fatalf("выключенный канал обязан сохраняться: %v", err)
	}
	if _, stored := repo.rows["fake"]; !stored {
		t.Fatal("строка не сохранена")
	}
}

// Контракт Descriptor.SecretField ↔ Kind == FieldSecret проверяется на сборке.
// Шифруется и вырезается из config_json только поле, названное в SecretField;
// второе поле с Kind == FieldSecret легло бы в config_json открытым текстом, и
// снаружи этого не было бы видно — API режет секретные поля перед выдачей, так
// что незашифрованный секрет в БД остался бы незамеченным.
func TestSecretFieldContractIsEnforcedAtAssembly(t *testing.T) {
	mk := func(secretField string, fields ...notifychannel.Field) notifychannel.Channel {
		return notifychannel.Channel{
			Descriptor: notifychannel.Descriptor{
				Name: "fake", Title: "Фейковый", SecretField: secretField, Fields: fields,
			},
			New: func(notifychannel.Deps) (notifychannel.Sender, error) {
				return &recordingSender{}, nil
			},
		}
	}
	url := notifychannel.Field{Key: "base_url", Label: "URL", Required: true, Kind: notifychannel.FieldURL}
	token := notifychannel.Field{Key: "token", Label: "Токен", Kind: notifychannel.FieldSecret}
	second := notifychannel.Field{Key: "token2", Label: "Второй токен", Kind: notifychannel.FieldSecret}

	bad := map[string]notifychannel.Channel{
		"два секретных поля":                  mk("token", url, token, second),
		"SecretField называет не то поле":     mk("base_url", url, token),
		"SecretField пуст при секретном поле": mk("", url, token),
		"SecretField без секретного поля":     mk("token", url),
	}
	for name, ch := range bad {
		t.Run(name, func(t *testing.T) {
			if _, err := notificationchannelsvc.New(&fakeRepo{}, newKey(t),
				[]notifychannel.Channel{ch}, gate{}, grantedOnly(), nil); err == nil {
				t.Fatal("сборка обязана отвергать нарушение контракта секретного поля")
			}
		})
	}

	// Согласованный дескриптор и дескриптор вовсе без секрета собираются как прежде.
	for name, ch := range map[string]notifychannel.Channel{
		"согласованный": mk("token", url, token),
		"без секрета":   mk("", url),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := notificationchannelsvc.New(&fakeRepo{}, newKey(t),
				[]notifychannel.Channel{ch}, gate{}, grantedOnly(), nil); err != nil {
				t.Fatalf("корректный дескриптор отвергнут: %v", err)
			}
		})
	}
}

// Отказ конструктора канала — это «сохранённая конфигурация непригодна», а не
// сбой сервера: ядро помечает его ErrInvalidConfig, чтобы API мог ответить 422,
// и оставляет исходную ошибку достижимой через errors.Is.
func TestSenderMarksConstructorFailureAsInvalidConfig(t *testing.T) {
	repo := &fakeRepo{rows: map[string]notificationchannels.Config{}}
	broken := notifychannel.Channel{
		Descriptor: notifychannel.Descriptor{Name: "broken2", Title: "Сломанный"},
		New: func(notifychannel.Deps) (notifychannel.Sender, error) {
			return nil, notifychannel.ErrMissingSecret
		},
	}
	svc, err := notificationchannelsvc.New(repo, newKey(t),
		[]notifychannel.Channel{broken},
		gate{allow: map[string]bool{"entitlement.notifications.broken2": true}}, grantedOnly("broken2"), nil)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx := context.Background()
	if err := svc.Save(ctx, scope, notificationchannelsvc.SaveInput{Channel: "broken2"}, 1); err != nil {
		t.Fatalf("save: %v", err)
	}
	_, senderErr := svc.Sender(ctx, scope, "broken2")
	if !errors.Is(senderErr, notificationchannelsvc.ErrInvalidConfig) {
		t.Fatalf("got %v, want ErrInvalidConfig", senderErr)
	}
	if !errors.Is(senderErr, notifychannel.ErrMissingSecret) {
		t.Fatalf("исходная ошибка канала потеряна: %v", senderErr)
	}
}

// --- Реестр живых каналов, редактирующий логгер и выгрузка накопленного ---

// capturingHandler собирает записи лога: канал, который держит сообщения,
// сообщает об отказе доставки только сюда.
type capturingHandler struct {
	records []slog.Record
}

func (h *capturingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *capturingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(string) slog.Handler      { return h }

func (h *capturingHandler) all() string {
	var b strings.Builder
	for _, r := range h.records {
		b.WriteString(r.Message)
		r.Attrs(func(a slog.Attr) bool {
			b.WriteString(" ")
			b.WriteString(a.Key)
			b.WriteString("=")
			b.WriteString(a.Value.String())
			return true
		})
		b.WriteString("\n")
	}
	return b.String()
}

func newSvcWithLogger(t *testing.T, built **recordingSender, h slog.Handler) (*notificationchannelsvc.Service, *fakeRepo) {
	t.Helper()
	repo := &fakeRepo{rows: map[string]notificationchannels.Config{}}
	svc, err := notificationchannelsvc.New(repo, newKey(t),
		[]notifychannel.Channel{testChannel(built)},
		gate{allow: entitledAndGranted}, grantedOnly("fake"), slog.New(h))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	return svc, repo
}

func saveFake(t *testing.T, svc *notificationchannelsvc.Service, baseURL, secret string) {
	t.Helper()
	if err := svc.Save(context.Background(), scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: true,
		Values: map[string]any{"base_url": baseURL}, Secret: secret,
	}, 1); err != nil {
		t.Fatalf("save: %v", err)
	}
}

// Экземпляр канала переживает вызовы Sender: канал, который копит сообщения,
// держит их внутри себя, поэтому пересборка на каждое обращение создавала бы
// буфер и тут же его выбрасывала. Кэш идентификатора бота в Mattermost — та же
// история этажом ниже: он писался ради переиспользования, которого не было.
func TestSenderReusesTheLiveChannelInstance(t *testing.T) {
	var built *recordingSender
	svc, _ := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)
	ctx := context.Background()
	saveFake(t, svc, "https://mm", "секрет-1")

	first, err := svc.Sender(ctx, scope, "fake")
	if err != nil {
		t.Fatalf("sender 1: %v", err)
	}
	instance := built

	second, err := svc.Sender(ctx, scope, "fake")
	if err != nil {
		t.Fatalf("sender 2: %v", err)
	}
	if built != instance {
		t.Fatal("конструктор канала вызван повторно при неизменившейся конфигурации")
	}
	if first != second {
		t.Fatal("Sender обязан отдавать тот же экземпляр при неизменившейся конфигурации")
	}
}

// Изменение настроек администратором обязано и заменить экземпляр, и выгрузить
// накопленное прежним — иначе сообщения, принятые до правки, молча пропадают.
func TestChangedSettingsRebuildTheChannelAndFlushWhatItHeld(t *testing.T) {
	var built *recordingSender
	svc, _ := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)
	ctx := context.Background()
	saveFake(t, svc, "https://mm", "секрет-1")

	sender, err := svc.Sender(ctx, scope, "fake")
	if err != nil {
		t.Fatalf("sender: %v", err)
	}
	if err := sender.Send(ctx, notifychannel.Target{Email: "a@b.c"}, notifychannel.Message{Title: "t"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	old := built

	saveFake(t, svc, "https://mm-2", "секрет-1")
	if _, err := svc.Sender(ctx, scope, "fake"); err != nil {
		t.Fatalf("sender после смены настроек: %v", err)
	}

	if built == old {
		t.Fatal("канал не пересобран после изменения настроек")
	}
	if old.flushed != 1 {
		t.Fatalf("накопленное прежним экземпляром не выгружено перед заменой: flushed=%d", old.flushed)
	}
	if built.built.Values["base_url"] != "https://mm-2" {
		t.Fatalf("новый экземпляр собран по старым настройкам: %+v", built.built.Values)
	}
}

// Смена одного лишь секрета — тоже смена конфигурации: канал, продолжающий
// работать на отозванном токене, это тот отказ, ради которого отпечаток берётся
// со всей строки, а не с одного времени изменения.
func TestChangedSecretAloneRebuildsTheChannel(t *testing.T) {
	var built *recordingSender
	svc, _ := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)
	ctx := context.Background()
	saveFake(t, svc, "https://mm", "секрет-1")
	if _, err := svc.Sender(ctx, scope, "fake"); err != nil {
		t.Fatalf("sender: %v", err)
	}
	old := built

	saveFake(t, svc, "https://mm", "секрет-2")
	if _, err := svc.Sender(ctx, scope, "fake"); err != nil {
		t.Fatalf("sender: %v", err)
	}
	if built == old {
		t.Fatal("канал не пересобран после смены секрета")
	}
	if built.built.Secret != "секрет-2" {
		t.Fatalf("новый экземпляр получил прежний секрет: %q", built.built.Secret)
	}
}

// Выгрузка при остановке: канал копит в памяти, и штатный выход — единственный
// шанс накопленного уйти.
func TestFlushDeliversWhatEveryLiveChannelHolds(t *testing.T) {
	var built *recordingSender
	svc, _ := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)
	ctx := context.Background()
	saveFake(t, svc, "https://mm", "секрет-1")

	if _, err := svc.Sender(ctx, scope, "fake"); err != nil {
		t.Fatalf("sender: %v", err)
	}
	if err := svc.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if built.flushed != 1 {
		t.Fatalf("живой канал не выгружен при остановке: flushed=%d", built.flushed)
	}
}

// Выгружать нечего, пока ни один канал не построен: остановка приложения без
// настроенных каналов обязана проходить молча.
func TestFlushWithoutLiveChannelsIsANoop(t *testing.T) {
	var built *recordingSender
	svc, _ := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)
	if err := svc.Flush(context.Background()); err != nil {
		t.Fatalf("выгрузка без живых каналов: %v", err)
	}
}

// Немедленная отправка возвращает ошибку прямо в ответ администратору, поэтому
// маскирующая обёртка обязана переопределять и её. Встраивание интерфейса
// пропустило бы её насквозь — молча, без ошибки компиляции, и секрет ушёл бы
// наружу ровно там, где это опаснее всего.
func TestSendNowErrorIsMaskedToo(t *testing.T) {
	const secret = "секрет-в-url-4821"
	upstream := fmt.Errorf("telegram: request to https://api.telegram.org/bot%s/sendMessage failed: %w", secret, errUpstream)

	repo := &fakeRepo{rows: map[string]notificationchannels.Config{}}
	svc, err := notificationchannelsvc.New(repo, newKey(t),
		[]notifychannel.Channel{failingChannel("failing", "token", upstream)},
		gate{allow: map[string]bool{"entitlement.notifications.failing": true}}, grantedOnly("failing"), nil)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx := context.Background()
	if err := svc.Save(ctx, scope, notificationchannelsvc.SaveInput{
		Channel: "failing", Enabled: true, Values: map[string]any{"base_url": "https://x"}, Secret: secret,
	}, 1); err != nil {
		t.Fatalf("save: %v", err)
	}
	sender, err := svc.Sender(ctx, scope, "failing")
	if err != nil {
		t.Fatalf("sender: %v", err)
	}

	sendNowErr := sender.SendNow(ctx, notifychannel.Target{Email: "a@b.c"}, notifychannel.Message{Title: "t"})
	if sendNowErr == nil {
		t.Fatal("ожидалась ошибка немедленной отправки")
	}
	if strings.Contains(sendNowErr.Error(), secret) {
		t.Fatalf("секрет утёк в ответе проверочной отправки: %s", sendNowErr.Error())
	}
	if !errors.Is(sendNowErr, errUpstream) {
		t.Fatal("маскировка обязана сохранить цепочку errors.Is до исходной ошибки канала")
	}

	flushErr := sender.Flush(ctx)
	if flushErr == nil {
		t.Fatal("ожидалась ошибка выгрузки")
	}
	if strings.Contains(flushErr.Error(), secret) {
		t.Fatalf("секрет утёк в ошибке выгрузки: %s", flushErr.Error())
	}
}

// Канал получает логгер, который не может записать секрет: как только канал
// начинает писать ошибки сам, обёртка вокруг Send перестаёт быть второй линией
// защиты, и редактирование обязано жить в обработчике.
func TestChannelGetsALoggerThatCannotWriteTheSecret(t *testing.T) {
	const secret = "секрет-в-url-4821"
	h := &capturingHandler{}
	var built *recordingSender
	svc, _ := newSvcWithLogger(t, &built, h)
	ctx := context.Background()
	saveFake(t, svc, "https://mm", secret)

	if _, err := svc.Sender(ctx, scope, "fake"); err != nil {
		t.Fatalf("sender: %v", err)
	}
	logger := built.deps.Log()
	if logger == nil {
		t.Fatal("канал обязан получить логгер")
	}

	logger.Error("доставка не удалась: https://api.example.com/bot" + secret + "/send")
	logger.Error("ошибка канала", "err", fmt.Errorf("token %s rejected", secret))
	logger.Error("во вложенной группе", slog.Group("req", slog.String("url", "https://x/"+secret)))

	text := h.all()
	if strings.Contains(text, secret) {
		t.Fatalf("секрет попал в журнал: %s", text)
	}
	if !strings.Contains(text, "доставка не удалась") || !strings.Contains(text, "rejected") {
		t.Fatalf("редактирование съело диагностику: %s", text)
	}
}

// Часы канала берутся из сервиса: канал, владеющий окном отправки, обязан
// получить их, а не читать время сам.
func TestChannelGetsAClock(t *testing.T) {
	var built *recordingSender
	svc, _ := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)
	ctx := context.Background()
	saveFake(t, svc, "https://mm", "секрет-1")
	if _, err := svc.Sender(ctx, scope, "fake"); err != nil {
		t.Fatalf("sender: %v", err)
	}
	if built.deps.Clock() == nil {
		t.Fatal("канал обязан получить пригодные часы")
	}
}

// --- Находки код-ревью ---

// Сохранение настроек обязано снимать живой экземпляр и выгружать накопленное
// СРАЗУ, а не оставлять это следующей доставке.
//
// Замена в live() ленивая, и этого хватает, только пока канал продолжают
// спрашивать. Выключенный администратором канал больше не спрашивают никогда:
// экземпляр остаётся в реестре, а его таймер окна всё равно срабатывает и шлёт
// накопленное по уже отозванным настройкам.
func TestSaveRetiresTheLiveInstanceAndFlushesIt(t *testing.T) {
	var built *recordingSender
	svc, _ := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)
	ctx := context.Background()
	saveFake(t, svc, "https://mm", "секрет-1")

	sender, err := svc.Sender(ctx, scope, "fake")
	if err != nil {
		t.Fatalf("sender: %v", err)
	}
	if err := sender.Send(ctx, notifychannel.Target{Email: "a@b.c"}, notifychannel.Message{Title: "t"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	old := built

	// Администратор выключает канал. Больше его никто не спросит.
	if err := svc.Save(ctx, scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: false, Values: map[string]any{"base_url": "https://mm"},
	}, 1); err != nil {
		t.Fatalf("save (disable): %v", err)
	}

	if old.flushed != 1 {
		t.Fatalf("накопленное не выгружено при сохранении: flushed=%d", old.flushed)
	}
	// Экземпляр снят: выгрузка при остановке приложения больше его не увидит,
	// и повторной отправки того же буфера не будет.
	if err := svc.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if old.flushed != 1 {
		t.Fatalf("снятый экземпляр всё ещё в реестре: flushed=%d", old.flushed)
	}
}

// После сохранения следующий запрос собирает НОВЫЙ экземпляр — по новым
// настройкам, а не по тем, что были до правки.
func TestSaveMakesTheNextSenderANewInstance(t *testing.T) {
	var built *recordingSender
	svc, _ := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)
	ctx := context.Background()
	saveFake(t, svc, "https://mm", "секрет-1")
	if _, err := svc.Sender(ctx, scope, "fake"); err != nil {
		t.Fatalf("sender: %v", err)
	}
	old := built

	saveFake(t, svc, "https://mm-2", "секрет-1")
	if _, err := svc.Sender(ctx, scope, "fake"); err != nil {
		t.Fatalf("sender после сохранения: %v", err)
	}
	if built == old {
		t.Fatal("после сохранения переиспользован прежний экземпляр")
	}
	if built.built.Values["base_url"] != "https://mm-2" {
		t.Fatalf("новый экземпляр собран по старым настройкам: %+v", built.built.Values)
	}
}

// loggingChannel пишет в выданный ему логгер и отказывается собираться. Так
// ведёт себя канал из чужого репозитория, объясняющий, почему настройки не
// приняты, — и ровно тут он может выписать наружу секрет.
func loggingChannel(name string) notifychannel.Channel {
	return notifychannel.Channel{
		Descriptor: notifychannel.Descriptor{
			Name: name, Title: "Болтливый", SecretField: "token",
			Fields: []notifychannel.Field{
				{Key: "base_url", Label: "URL", Required: true, Kind: notifychannel.FieldURL},
				{Key: "token", Label: "Токен", Required: true, Kind: notifychannel.FieldSecret},
			},
		},
		New: func(d notifychannel.Deps) (notifychannel.Sender, error) {
			d.Log().Error("не могу собраться", "token", d.Settings.Secret)
			return nil, errors.New("отказ конструктора")
		},
	}
}

// Проверочная сборка на пути сохранения получает секрет в открытом виде.
// Логгер ей обязан достаться такой же отредактированный, как и живому каналу:
// иначе канал, объясняющий отказ, выпишет токен в журнал при обычном сохранении.
func TestSaveProbeGetsAScrubbedLoggerToo(t *testing.T) {
	const secret = "секрет-в-логе-4821"
	h := &capturingHandler{}
	repo := &fakeRepo{rows: map[string]notificationchannels.Config{}}
	svc, err := notificationchannelsvc.New(repo, newKey(t),
		[]notifychannel.Channel{loggingChannel("noisy")},
		gate{allow: map[string]bool{"entitlement.notifications.noisy": true}},
		grantedOnly("noisy"), slog.New(h))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	saveErr := svc.Save(context.Background(), scope, notificationchannelsvc.SaveInput{
		Channel: "noisy", Enabled: true,
		Values: map[string]any{"base_url": "https://x"}, Secret: secret,
	}, 1)
	if !errors.Is(saveErr, notificationchannelsvc.ErrInvalidConfig) {
		t.Fatalf("got %v, want ErrInvalidConfig", saveErr)
	}
	if text := h.all(); strings.Contains(text, secret) {
		t.Fatalf("секрет утёк в журнал при сохранении: %s", text)
	}
	if !strings.Contains(h.all(), "не могу собраться") {
		t.Fatalf("редактирование съело диагностику: %s", h.all())
	}
}

// Конструирование сериализовано по каналу, и строка перечитывается внутри этой
// сериализации. Без этого два одновременных вызова собирают из РАЗНЫХ версий
// настроек, и побеждает тот, кто закончил последним, — а это не тот, кто читал
// более свежую строку. Отставший установил бы свой экземпляр поверх собранного
// по новым настройкам, и реестр продолжил бы работать на отозванных данных.
func TestConcurrentSendersNeverEndUpOnStaleSettings(t *testing.T) {
	var built *recordingSender
	svc, repo := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)
	ctx := context.Background()
	saveFake(t, svc, "https://old", "секрет-1")

	// Половина горутин стартует на старой строке, половина — после правки.
	// Кто бы ни закончил последним, реестр обязан остаться на новой.
	const n = 12
	start := make(chan struct{})
	done := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			<-start
			if i == n/2 {
				// Правка настроек в середине волны.
				repo.bump("fake", map[string]any{"base_url": "https://new"})
			}
			_, err := svc.Sender(ctx, scope, "fake")
			done <- err
		}(i)
	}
	close(start)
	for i := 0; i < n; i++ {
		if err := <-done; err != nil {
			t.Fatalf("sender %d: %v", i, err)
		}
	}

	// Последний собранный экземпляр обязан нести новые настройки: перечитывание
	// внутри сериализации не оставляет отставшему шанса победить.
	final, err := svc.Sender(ctx, scope, "fake")
	if err != nil {
		t.Fatalf("sender: %v", err)
	}
	if final == nil {
		t.Fatal("sender пуст")
	}
	if built.built.Values["base_url"] != "https://new" {
		t.Fatalf("реестр остался на старых настройках: %+v", built.built.Values)
	}
}

// Снятие экземпляра не должно давать отставшему конструктору установить свой
// результат в освободившийся слот: retire ждёт конструирование, а всё, что
// стартует после него, читает уже записанную строку.
func TestRetireDoesNotRaceAnInFlightConstruction(t *testing.T) {
	var built *recordingSender
	svc, _ := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)
	ctx := context.Background()
	saveFake(t, svc, "https://old", "секрет-1")
	if _, err := svc.Sender(ctx, scope, "fake"); err != nil {
		t.Fatalf("sender: %v", err)
	}

	const n = 8
	start := make(chan struct{})
	done := make(chan struct{}, n+1)
	for i := 0; i < n; i++ {
		go func() {
			<-start
			_, _ = svc.Sender(ctx, scope, "fake")
			done <- struct{}{}
		}()
	}
	go func() {
		<-start
		saveFake(t, svc, "https://new", "секрет-1") // внутри — retire
		done <- struct{}{}
	}()
	close(start)
	for i := 0; i < n+1; i++ {
		<-done
	}

	// Никакой экземпляр не мог остаться от строки, которой уже нет.
	if _, err := svc.Sender(ctx, scope, "fake"); err != nil {
		t.Fatalf("sender после гонки: %v", err)
	}
	if built.built.Values["base_url"] != "https://new" {
		t.Fatalf("после снятия реестр держит старые настройки: %+v", built.built.Values)
	}
}

// Выгрузка вытесненного экземпляра — сетевая операция с 30-секундным бюджетом.
// Держать на ней замок конструирования нельзя: сохранение настроек берёт тот же
// замок безусловно, и недоступный сервер останавливал бы админку на полминуты.
//
// Проверяется именно сохранением, а не обычной доставкой: у доставки есть
// быстрый путь по отпечатку, и она проходит мимо замка, даже когда он занят.
func TestFlushOfTheReplacedInstanceDoesNotBlockSaving(t *testing.T) {
	var built *recordingSender
	svc, repo := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)
	ctx := context.Background()
	saveFake(t, svc, "https://old", "секрет-1")
	if _, err := svc.Sender(ctx, scope, "fake"); err != nil {
		t.Fatalf("sender: %v", err)
	}

	// Вытесняемый экземпляр застрянет в выгрузке и сообщит, что она началась.
	gate := make(chan struct{})
	entered := make(chan struct{}, 1)
	built.flushGate, built.flushEntered = gate, entered

	repo.bump("fake", map[string]any{"base_url": "https://new"})

	replacing := make(chan error, 1)
	go func() {
		_, err := svc.Sender(ctx, scope, "fake")
		replacing <- err
	}()

	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		close(gate)
		t.Fatal("выгрузка вытесненного экземпляра так и не началась")
	}

	// Выгрузка идёт. Администратор сохраняет настройки — это обязано пройти.
	saving := make(chan error, 1)
	go func() {
		saving <- svc.Save(ctx, scope, notificationchannelsvc.SaveInput{
			Channel: "fake", Enabled: true,
			Values: map[string]any{"base_url": "https://newer"}, Secret: "секрет-2",
		}, 1)
	}()

	select {
	case err := <-saving:
		if err != nil {
			close(gate)
			t.Fatalf("сохранение: %v", err)
		}
	case <-time.After(3 * time.Second):
		close(gate)
		t.Fatal("сохранение заблокировано выгрузкой — замок конструирования удерживается во время сетевой операции")
	}

	close(gate)
	if err := <-replacing; err != nil {
		t.Fatalf("пересборка: %v", err)
	}
}

// Канал, выключенный администратором, доставке не отдаётся — даже если список
// каналов, с которым она пришла, был составлен мгновением раньше. Иначе лукап
// собрал бы свежий экземпляр и накопил в него по-настоящему новую работу, а
// таймер окна её отправил.
func TestDeliverySenderRefusesADisabledChannel(t *testing.T) {
	var built *recordingSender
	svc, _ := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)
	ctx := context.Background()
	saveFake(t, svc, "https://mm", "секрет-1")

	// Администратор выключает канал.
	if err := svc.Save(ctx, scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: false, Values: map[string]any{"base_url": "https://mm"},
	}, 1); err != nil {
		t.Fatalf("save (disable): %v", err)
	}

	if _, err := svc.DeliverySender(ctx, scope, "fake"); !errors.Is(err, notificationchannelsvc.ErrNotEnabled) {
		t.Fatalf("got %v, want ErrNotEnabled", err)
	}
}

// Проверочная отправка выключенный канал принимает: убедиться в настройках до
// того, как включить его всем, — законное действие, и сообщение уходит только
// тому администратору, который его запросил.
func TestProbeSenderStillWorksForADisabledChannel(t *testing.T) {
	var built *recordingSender
	svc, _ := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)
	ctx := context.Background()
	saveFake(t, svc, "https://mm", "секрет-1")
	if err := svc.Save(ctx, scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: false, Values: map[string]any{"base_url": "https://mm"},
	}, 1); err != nil {
		t.Fatalf("save (disable): %v", err)
	}

	if _, err := svc.Sender(ctx, scope, "fake"); err != nil {
		t.Fatalf("проверочная отправка обязана работать на выключенном канале: %v", err)
	}
}

// Гонка «выключили между выбором канала и лукапом» закрывается именно
// перечитыванием под замком сборки: строка, прочитанная вызывающим, могла
// устареть, и авторитетная перечитка обязана проверять признак заново.
func TestDisableBetweenSelectionAndLookupIsCaughtByTheAuthoritativeReread(t *testing.T) {
	var built *recordingSender
	svc, repo := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)
	ctx := context.Background()
	saveFake(t, svc, "https://mm", "секрет-1")

	// Строка выключается в обход сервиса — как это сделала бы другая реплика
	// между тем, как доставка выбрала канал, и тем, как она просит отправителя.
	row := repo.rows["fake"]
	row.Enabled = false
	repo.rows["fake"] = row

	if _, err := svc.DeliverySender(ctx, scope, "fake"); !errors.Is(err, notificationchannelsvc.ErrNotEnabled) {
		t.Fatalf("got %v, want ErrNotEnabled", err)
	}
}

// Экземпляр, уже отданный наружу, после снятия отказывается принимать сообщения.
//
// Доставка спрашивает отправителя один раз на пачку и дальше шлёт по этой ссылке
// сообщение за сообщением. Если администратор сохранит канал посреди пачки,
// снятие само по себе до этой ссылки не дотягивается: оно убирает экземпляр из
// реестра, но в руках у доставки он остаётся. Принятое им уже никто не выгрузит —
// выгрузка прошла, — а таймер окна всё равно выстрелит и отправит по настройкам,
// которых больше нет.
func TestRetiredSenderRefusesMessagesTheCallerStillTriesToSend(t *testing.T) {
	var built *recordingSender
	svc, _ := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)
	ctx := context.Background()
	saveFake(t, svc, "https://mm", "секрет-1")

	sender, err := svc.DeliverySender(ctx, scope, "fake")
	if err != nil {
		t.Fatalf("sender: %v", err)
	}
	if err := sender.Send(ctx, notifychannel.Target{Email: "a@b.c"}, notifychannel.Message{Title: "первое"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	old := built

	// Администратор выключает канал, пока пачка ещё не дошла до конца.
	if err := svc.Save(ctx, scope, notificationchannelsvc.SaveInput{
		Channel: "fake", Enabled: false, Values: map[string]any{"base_url": "https://mm"},
	}, 1); err != nil {
		t.Fatalf("save (disable): %v", err)
	}

	// Та же ссылка, следующее сообщение пачки.
	err = sender.Send(ctx, notifychannel.Target{Email: "a@b.c"}, notifychannel.Message{Title: "второе"})
	if !errors.Is(err, notificationchannelsvc.ErrRetired) {
		t.Fatalf("снятый экземпляр принял сообщение: err=%v", err)
	}
	if old.accepted != 1 {
		t.Fatalf("сообщение всё-таки дошло до канала: accepted=%d", old.accepted)
	}
	// SendNow — тот же запрет: проверочная отправка тоже не должна уходить по
	// снятым настройкам.
	if err := sender.SendNow(ctx, notifychannel.Target{Email: "a@b.c"}, notifychannel.Message{Title: "проверка"}); !errors.Is(err, notificationchannelsvc.ErrRetired) {
		t.Fatalf("снятый экземпляр выполнил немедленную отправку: err=%v", err)
	}
	if old.sentNow != 0 {
		t.Fatalf("немедленная отправка дошла до канала: sentNow=%d", old.sentNow)
	}
}

// Отзыв обязан произойти ДО выгрузки, а не после неё.
//
// Порядок здесь — это и есть гарантия. Если сначала выгрузить, а отозвать потом,
// остаётся окно, в котором сообщение попадает в буфер уже выгруженного
// экземпляра: выгрузка его не застала, и достать его оттуда сможет только таймер
// окна — по тем самым настройкам, которые только что сменили.
func TestRetirementRevokesBeforeItFlushes(t *testing.T) {
	var built *recordingSender
	svc, _ := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)
	ctx := context.Background()
	saveFake(t, svc, "https://mm", "секрет-1")

	sender, err := svc.DeliverySender(ctx, scope, "fake")
	if err != nil {
		t.Fatalf("sender: %v", err)
	}
	old := built

	// Снимаемый экземпляр застрянет в выгрузке и сообщит, что она началась.
	gate := make(chan struct{})
	entered := make(chan struct{}, 1)
	old.flushGate, old.flushEntered = gate, entered

	saving := make(chan error, 1)
	go func() {
		saving <- svc.Save(ctx, scope, notificationchannelsvc.SaveInput{
			Channel: "fake", Enabled: false, Values: map[string]any{"base_url": "https://mm"},
		}, 1)
	}()

	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		close(gate)
		t.Fatal("выгрузка снимаемого экземпляра так и не началась")
	}

	// Выгрузка идёт. Сообщение по старой ссылке обязано быть отвергнуто уже
	// сейчас — иначе оно ляжет в буфер, который эта выгрузка уже не заберёт.
	err = sender.Send(ctx, notifychannel.Target{Email: "a@b.c"}, notifychannel.Message{Title: "во время выгрузки"})
	close(gate)
	if !errors.Is(err, notificationchannelsvc.ErrRetired) {
		t.Fatalf("экземпляр принял сообщение во время собственной выгрузки: err=%v", err)
	}
	if err := <-saving; err != nil {
		t.Fatalf("сохранение: %v", err)
	}
	if old.accepted != 0 {
		t.Fatalf("сообщение осело в буфере после выгрузки: accepted=%d", old.accepted)
	}
}

// chattyChannel собирается успешно и пишет в логгер составное значение: структуру
// настроек целиком. Так канал из чужого репозитория выписывает наружу секрет, ни
// разу не передав его строкой, — slog.Any("settings", d.Settings) выглядит
// безобидно, а обработчик разложит структуру по полям.
func chattyChannel(name string, built **recordingSender) notifychannel.Channel {
	return notifychannel.Channel{
		Descriptor: notifychannel.Descriptor{
			Name: name, Title: "Составной", SecretField: "token",
			Fields: []notifychannel.Field{
				{Key: "base_url", Label: "URL", Required: true, Kind: notifychannel.FieldURL},
				{Key: "token", Label: "Токен", Required: true, Kind: notifychannel.FieldSecret},
			},
		},
		New: func(d notifychannel.Deps) (notifychannel.Sender, error) {
			s := &recordingSender{built: d.Settings, deps: d}
			*built = s
			return s, nil
		},
	}
}

// Секрет, спрятанный внутри составного значения, тоже редактируется.
//
// Проверка по slog.Kind недостаточна: структура, карта или срез приезжают одним
// KindAny, обёртка их не разбирает, а нижележащий обработчик разложит по полям и
// выпишет токен. Строковых веток для этого не существует — значение обязано быть
// отрисовано и проверено здесь.
func TestSecretNestedInACompositeValueIsScrubbedToo(t *testing.T) {
	const secret = "секрет-внутри-структуры-77"
	h := &capturingHandler{}
	var built *recordingSender
	repo := &fakeRepo{rows: map[string]notificationchannels.Config{}}
	svc, err := notificationchannelsvc.New(repo, newKey(t),
		[]notifychannel.Channel{chattyChannel("fake", &built)},
		gate{allow: entitledAndGranted}, grantedOnly("fake"), slog.New(h))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx := context.Background()
	saveFake(t, svc, "https://mm", secret)
	if _, err := svc.Sender(ctx, scope, "fake"); err != nil {
		t.Fatalf("sender: %v", err)
	}
	logger := built.deps.Log()

	// Настройки целиком: секрет лежит в экспортированном поле структуры.
	logger.Error("не приняты настройки", slog.Any("settings", built.deps.Settings))
	// Карта — ровно то же самое, другой формой.
	logger.Error("тело запроса", slog.Any("body", map[string]string{"token": secret}))
	// Срез значений.
	logger.Error("заголовки", slog.Any("headers", []string{"Authorization: Bearer " + secret}))
	// А это — составное значение БЕЗ секрета. Отрисовка здесь нужна только чтобы
	// найти секрет, и не должна подменять собой форматирование обработчика для
	// подавляющего большинства записей.
	logger.Error("повтор", slog.Any("state", struct{ Attempt int }{Attempt: 3}))

	text := h.all()
	if strings.Contains(text, secret) {
		t.Fatalf("секрет попал в журнал внутри составного значения: %s", text)
	}
	if !strings.Contains(text, "не приняты настройки") || !strings.Contains(text, "Authorization") {
		t.Fatalf("редактирование съело диагностику: %s", text)
	}
	// Именно "{3}", а не "{Attempt:3}": обработчик отрисовал структуру сам, своим
	// %v. Подменённое значение приехало бы строкой в форме %+v — так что это
	// отличает «пропустили как было» от «на всякий случай отрисовали и вставили».
	if !strings.Contains(text, "state={3}") {
		t.Fatalf("значение без секрета не доехало до обработчика как было: %s", text)
	}
}

// Реплика, не обрабатывавшая сохранение, новую работу в свой застоявшийся
// экземпляр не принимает.
//
// Это и есть граница того, чем ограничен отказ от межрепличной инвалидации.
// Реестр каждой реплики локальный: PUT настроек уходит на одну из них, и снятие
// экземпляра происходит только там. У остальных живой экземпляр остаётся — но
// достучаться до него новым сообщением нельзя.
//
// Держат это два независимых заслона, и тест проверяет свойство целиком, а не
// один из них: строка канала читается из базы на каждом запросе отправителя, и
// признак Enabled входит в отпечаток — поэтому выключение всегда промахивается
// мимо быстрого пути и попадает в авторитетную перечитку под замком сборки.
// Снятие любого одного заслона тест не заметит, снятие обоих — валит.
//
// Проверка именно с уже собранным экземпляром: без него отказ случился бы и так,
// и застоявшийся экземпляр в сценарии вообще бы не участвовал.
func TestLiveInstanceOnAnotherReplicaStopsTakingWorkOnceTheRowIsDisabled(t *testing.T) {
	var built *recordingSender
	svc, repo := newSvc(t, entitledAndGranted, grantedOnly("fake"), &built)
	ctx := context.Background()
	saveFake(t, svc, "https://mm", "секрет-1")

	// Эта реплика уже собрала экземпляр и держит его в реестре.
	if _, err := svc.DeliverySender(ctx, scope, "fake"); err != nil {
		t.Fatalf("sender: %v", err)
	}
	if built == nil {
		t.Fatal("экземпляр не собран — проверять нечего")
	}

	// Администратор выключает канал на ДРУГОЙ реплике: в базе строка меняется,
	// локальный реестр об этом не узнаёт и продолжает держать живой экземпляр.
	row := repo.rows["fake"]
	row.Enabled = false
	repo.rows["fake"] = row

	if _, err := svc.DeliverySender(ctx, scope, "fake"); !errors.Is(err, notificationchannelsvc.ErrNotEnabled) {
		t.Fatalf("застоявшийся экземпляр отдан доставке: err=%v", err)
	}
	if built.accepted != 0 {
		t.Fatalf("в застоявшийся экземпляр попала новая работа: accepted=%d", built.accepted)
	}
}
