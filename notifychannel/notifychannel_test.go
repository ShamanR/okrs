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
