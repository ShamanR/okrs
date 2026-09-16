package notifychannel_test

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"okrs/notifychannel"
)

// Пакет обязан оставаться контрактом без зависимостей: автор канала из чужого
// модуля получает только типы. Тест фиксирует, что Channel собирается из
// Descriptor и конструктора, и что Sender удовлетворяется обычной реализацией.
type fakeSender struct {
	accepted []notifychannel.Message
	sentNow  []notifychannel.Message
	flushed  int
}

func (f *fakeSender) Send(_ context.Context, _ notifychannel.Target, m notifychannel.Message) error {
	f.accepted = append(f.accepted, m)
	return nil
}

func (f *fakeSender) SendNow(_ context.Context, _ notifychannel.Target, m notifychannel.Message) error {
	f.sentNow = append(f.sentNow, m)
	return nil
}

func (f *fakeSender) Close(context.Context) error {
	f.flushed++
	return nil
}

// Ограничение пакета — не стилистика, а условие работоспособности: Go запрещает
// импорт okrs/internal/** из чужого модуля, поэтому канал, написанный снаружи,
// перестанет собираться в тот же день, когда контракт потянет за собой internal.
// Компилятор об этом не скажет — внутри этого модуля такой импорт легален.
func TestPackageHasNoInternalImports(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("чтение каталога пакета: %v", err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Clean(name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("разбор %s: %v", name, err)
		}
		checked++
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("разбор импорта в %s: %v", name, err)
			}
			if path == "okrs/internal" || strings.HasPrefix(path, "okrs/internal/") {
				t.Errorf("%s импортирует %q: контракт станет несобираемым из чужого модуля", name, path)
			}
		}
	}
	if checked == 0 {
		t.Fatal("не найдено ни одного файла пакета — тест ничего не проверил")
	}
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
		New: func(d notifychannel.Deps) (notifychannel.Sender, error) {
			if d.Settings.Secret == "" {
				return nil, notifychannel.ErrMissingSecret
			}
			return &fakeSender{}, nil
		},
	}

	if _, err := ch.New(notifychannel.Deps{}); err == nil {
		t.Fatal("конструктор обязан отвергать пустой секрет")
	}
	sender, err := ch.New(notifychannel.Deps{Settings: notifychannel.Settings{
		Secret: "s", Values: map[string]any{"base_url": "https://x"},
	}})
	if err != nil {
		t.Fatalf("конструктор: %v", err)
	}
	if err := sender.Send(context.Background(), notifychannel.Target{Email: "a@b.c"},
		notifychannel.Message{Title: "t", Body: "b"}); err != nil {
		t.Fatalf("send: %v", err)
	}
}

// Три метода Sender различаются тем, кто узнаёт результат. Тест фиксирует, что
// все три входят в сам интерфейс: необязательный интерфейс здесь непригоден —
// приведение к нему не проходит через обёртку, маскирующую секрет.
func TestSenderCarriesAllThreeMethods(t *testing.T) {
	f := &fakeSender{}
	var s notifychannel.Sender = f

	ctx := context.Background()
	msg := notifychannel.Message{Title: "t"}
	if err := s.Send(ctx, notifychannel.Target{Email: "a@b.c"}, msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := s.SendNow(ctx, notifychannel.Target{Email: "a@b.c"}, msg); err != nil {
		t.Fatalf("SendNow: %v", err)
	}
	if err := s.Close(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if len(f.accepted) != 1 || len(f.sentNow) != 1 || f.flushed != 1 {
		t.Fatalf("каждый метод обязан быть отдельным: accepted=%d sentNow=%d flushed=%d",
			len(f.accepted), len(f.sentNow), f.flushed)
	}
}

// Логгер и часы необязательны: канал, собранный без них, обязан работать, а не
// падать на разыменовании nil.
func TestDepsSubstituteMissingLoggerAndClock(t *testing.T) {
	var d notifychannel.Deps
	if d.Log() == nil {
		t.Fatal("Log обязан возвращать пригодный логгер при отсутствии заданного")
	}
	d.Log().Error("ничего не должно упасть")

	if d.Clock() == nil {
		t.Fatal("Clock обязан возвращать пригодные часы при отсутствии заданных")
	}
	if d.Clock()().IsZero() {
		t.Fatal("часы по умолчанию обязаны отдавать реальное время")
	}

	fixed := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	d.Now = func() time.Time { return fixed }
	if got := d.Clock()(); !got.Equal(fixed) {
		t.Fatalf("заданные часы обязаны использоваться: %v", got)
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
