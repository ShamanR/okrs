package test_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"okrs/internal/core/domain"
	adminnotifications "okrs/internal/http/handlers/api/v1/admin/settings/notifications"
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

// handlertest.User/UserID намеренно оставляют Email пустым (проверено по коду
// handlertest.go), а хендлер проверочной отправки адресует получателя именно по
// auth.UserFromContext(ctx).Email — поэтому здесь контекст собирается через
// handlertest.UserEmail с реальным адресом.

// Тестовое сообщение уходит тому, кто нажал кнопку, — и никому больше.
func TestSendsToTheCallerOnly(t *testing.T) {
	s := &fakeSender{}
	h := channeltest.New(&fakeSvc{sender: s})
	rec := handlertest.Do(h.Test, http.MethodPost,
		"/api/v1/admin/settings/notifications/mattermost/test", "",
		handlertest.Tenant(1), handlertest.UserEmail(1, "udid-42", "admin@example.com"),
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
		handlertest.Tenant(1), handlertest.UserEmail(1, "udid-42", "admin@example.com"),
		handlertest.URLParam("channel", "mattermost"))
	handlertest.Status(t, rec, http.StatusConflict)
}

// Недоступный канал — 404, теми же соображениями, что и в соседнем пакете.
func TestUnavailableChannelIs404(t *testing.T) {
	h := channeltest.New(&fakeSvc{err: notificationchannelsvc.ErrNotAvailable})
	rec := handlertest.Do(h.Test, http.MethodPost,
		"/api/v1/admin/settings/notifications/telegram/test", "",
		handlertest.Tenant(1), handlertest.UserEmail(1, "udid-42", "admin@example.com"),
		handlertest.URLParam("channel", "telegram"))
	handlertest.Status(t, rec, http.StatusNotFound)
}

// Неизвестный канал отвечает так же, как недоступный — тем же требованием
// неразличимости, что и на эндпоинте настроек: перебор имён на эндпоинте
// проверочной отправки не должен раскрывать каталог каналов продукта ни
// статусом, ни текстом.
func TestUnknownChannelIs404(t *testing.T) {
	h := channeltest.New(&fakeSvc{err: notificationchannelsvc.ErrUnknownChannel})
	rec := handlertest.Do(h.Test, http.MethodPost,
		"/api/v1/admin/settings/notifications/nope/test", "",
		handlertest.Tenant(1), handlertest.UserEmail(1, "udid-42", "admin@example.com"),
		handlertest.URLParam("channel", "nope"))
	handlertest.Status(t, rec, http.StatusNotFound)
}

// Совпадение статуса — недостаточная проверка: тела обоих ответов обязаны
// совпадать байт в байт, иначе различающийся текст сам становится оракулом.
func TestUnavailableAndUnknownChannelReturnIdenticalBodies(t *testing.T) {
	unavailable := channeltest.New(&fakeSvc{err: notificationchannelsvc.ErrNotAvailable})
	recUnavailable := handlertest.Do(unavailable.Test, http.MethodPost,
		"/api/v1/admin/settings/notifications/telegram/test", "",
		handlertest.Tenant(1), handlertest.UserEmail(1, "udid-42", "admin@example.com"),
		handlertest.URLParam("channel", "telegram"))
	handlertest.Status(t, recUnavailable, http.StatusNotFound)
	bodyUnavailable := handlertest.Body(recUnavailable)

	unknown := channeltest.New(&fakeSvc{err: notificationchannelsvc.ErrUnknownChannel})
	recUnknown := handlertest.Do(unknown.Test, http.MethodPost,
		"/api/v1/admin/settings/notifications/nope/test", "",
		handlertest.Tenant(1), handlertest.UserEmail(1, "udid-42", "admin@example.com"),
		handlertest.URLParam("channel", "nope"))
	handlertest.Status(t, recUnknown, http.StatusNotFound)
	bodyUnknown := handlertest.Body(recUnknown)

	if bodyUnavailable != bodyUnknown {
		t.Fatalf("тела ответов различаются: недоступный=%q неизвестный=%q", bodyUnavailable, bodyUnknown)
	}
}

// Ошибку доставки показываем как есть: это единственный способ для администратора
// узнать, что токен отозван или адрес сервера неверен.
func TestDeliveryFailureIsReportedToTheAdmin(t *testing.T) {
	h := channeltest.New(&fakeSvc{sender: &fakeSender{err: errors.New("mattermost: status 401")}})
	rec := handlertest.Do(h.Test, http.MethodPost,
		"/api/v1/admin/settings/notifications/mattermost/test", "",
		handlertest.Tenant(1), handlertest.UserEmail(1, "udid-42", "admin@example.com"),
		handlertest.URLParam("channel", "mattermost"))
	handlertest.Status(t, rec, http.StatusBadGateway)
	body := handlertest.Body(rec)
	if !strings.Contains(body, "401") {
		t.Fatalf("ответ не объясняет причину: %s", body)
	}
}

// Пользователь без email в профиле не может пройти проверку канала: адресовать
// сообщение некуда, а не «отправить и потерять».
func TestMissingEmailIsUnprocessable(t *testing.T) {
	h := channeltest.New(&fakeSvc{sender: &fakeSender{}})
	rec := handlertest.Do(h.Test, http.MethodPost,
		"/api/v1/admin/settings/notifications/mattermost/test", "",
		handlertest.Tenant(1), handlertest.User("udid-42"),
		handlertest.URLParam("channel", "mattermost"))
	handlertest.Status(t, rec, http.StatusUnprocessableEntity)
}

func TestRequiresTenantScope(t *testing.T) {
	h := channeltest.New(&fakeSvc{sender: &fakeSender{}})
	handlertest.RequiresTenantScope(t, h.Test, http.MethodPost,
		"/api/v1/admin/settings/notifications/mattermost/test",
		handlertest.URLParam("channel", "mattermost"))
}

// Кнопки «Сохранить» и «Проверить» стоят на одной карточке, поэтому на одну и ту
// же причину — снятый NOTIFICATIONS_SECRET_KEY — они обязаны отвечать одинаково.
// Раньше «Проверить» отдавала 500 «не удалось подготовить канал», то есть
// оператор, сам снявший переменную, узнавал причину только от соседней кнопки.
func TestMissingSecretKeyIsServiceUnavailableWithTheSameWordingAsSave(t *testing.T) {
	h := channeltest.New(&fakeSvc{err: notificationchannelsvc.ErrNoSecretKey})
	rec := handlertest.Do(h.Test, http.MethodPost,
		"/api/v1/admin/settings/notifications/mattermost/test", "",
		handlertest.Tenant(1), handlertest.UserEmail(1, "udid-42", "admin@example.com"),
		handlertest.URLParam("channel", "mattermost"))
	handlertest.Status(t, rec, http.StatusServiceUnavailable)
	if !strings.Contains(handlertest.Body(rec), adminnotifications.NoSecretKeyMessage) {
		t.Fatalf("текст обязан совпадать с текстом сохранения: %s", handlertest.Body(rec))
	}
}

// Канал отверг сохранённые настройки — это неполная конфигурация, которую
// администратор чинит здесь же, а не сбой сервера: 422, а не 500.
func TestRejectedConfigurationIsUnprocessable(t *testing.T) {
	h := channeltest.New(&fakeSvc{
		err: fmt.Errorf("notificationchannel: fake: %w: %w",
			notificationchannelsvc.ErrInvalidConfig, notifychannel.ErrMissingSecret),
	})
	rec := handlertest.Do(h.Test, http.MethodPost,
		"/api/v1/admin/settings/notifications/mattermost/test", "",
		handlertest.Tenant(1), handlertest.UserEmail(1, "udid-42", "admin@example.com"),
		handlertest.URLParam("channel", "mattermost"))
	handlertest.Status(t, rec, http.StatusUnprocessableEntity)
}

// Администратор пространства сам задаёт base_url, поэтому сырая транспортная
// ошибка превращает кнопку «Проверить» в сканер внутренней сети с оракулом:
// по ответу различимы открытый порт, закрытый порт и статус HTTP для любого
// внутрикластерного адреса. Наружу уходит общий текст без единой детали адреса.
func TestTransportFailureDoesNotLeakTheAddress(t *testing.T) {
	transport := fmt.Errorf("mattermost: /api/v4/users/me: %w", &url.Error{
		Op:  "Get",
		URL: "http://10.0.0.5:8065/api/v4/users/me",
		Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: connection refused")},
	})
	h := channeltest.New(&fakeSvc{sender: &fakeSender{err: transport}})
	rec := handlertest.Do(h.Test, http.MethodPost,
		"/api/v1/admin/settings/notifications/mattermost/test", "",
		handlertest.Tenant(1), handlertest.UserEmail(1, "udid-42", "admin@example.com"),
		handlertest.URLParam("channel", "mattermost"))
	handlertest.Status(t, rec, http.StatusBadGateway)

	body := handlertest.Body(rec)
	for _, leak := range []string{"10.0.0.5", "8065", "connection refused", "dial"} {
		if strings.Contains(body, leak) {
			t.Fatalf("ответ выносит наружу деталь адреса %q: %s", leak, body)
		}
	}
	if !strings.Contains(body, "подключиться") {
		t.Fatalf("ответ не объясняет администратору, что случилось: %s", body)
	}
}
