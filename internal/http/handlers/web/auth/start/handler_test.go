package start

// Обработчику нужен настоящий auth.Manager: он спрашивает у него провайдера
// по имени из пути, и на nil-менеджере тест упал бы паникой вместо проверки.
// В disabled-режиме провайдеров нет — ровно то, что нужно для ветки «unknown».

import (
	"net/http"
	"testing"

	authpkg "okrs/internal/auth"
	"okrs/internal/http/handlers/handlertest"
)

func manager(t *testing.T) *authpkg.Manager {
	t.Helper()
	mgr, err := authpkg.NewManager(authpkg.Config{Mode: authpkg.ModeDisabled}, nil)
	if err != nil {
		t.Fatalf("auth.NewManager: %v", err)
	}
	return mgr
}

// Незарегистрированный провайдер — ошибка клиента: адрес набран руками или
// провайдер отключён в конфиге. 400, а не редирект в никуда.
func TestUnknownProviderIs400(t *testing.T) {
	w := handlertest.Do(New(manager(t)).Get, http.MethodGet, "/auth/нетакого/start", "",
		handlertest.URLParam("provider", "нетакого"))
	handlertest.Status(t, w, http.StatusBadRequest)
}

func TestEmptyProviderIs400(t *testing.T) {
	w := handlertest.Do(New(manager(t)).Get, http.MethodGet, "/auth//start", "")
	handlertest.Status(t, w, http.StatusBadRequest)
}
