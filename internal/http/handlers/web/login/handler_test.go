package login

// Страница входа: при единственном провайдере выбирать не из чего, поэтому
// пользователя сразу уводит на него; иначе рисуется чуузер.

import (
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authpkg "okrs/internal/auth"
)

func manager(t *testing.T) *authpkg.Manager {
	t.Helper()
	mgr, err := authpkg.NewManager(authpkg.Config{Mode: authpkg.ModeDisabled}, nil)
	if err != nil {
		t.Fatalf("auth.NewManager: %v", err)
	}
	return mgr
}

func loginTmpl(t *testing.T) *template.Template {
	t.Helper()
	root := template.New("root")
	if _, err := root.New("login").Parse(`страница входа next={{.Next}}`); err != nil {
		t.Fatalf("parse: %v", err)
	}
	return root
}

func logger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// В disabled-режиме провайдеров нет — редиректить некуда, рисуется страница.
func TestRendersChooserWhenNotExactlyOneProvider(t *testing.T) {
	w := httptest.NewRecorder()
	New(manager(t), loginTmpl(t), logger()).Get(w, httptest.NewRequest(http.MethodGet, "/login", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("код = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "страница входа") {
		t.Fatalf("отрендерилось не то: %q", w.Body.String())
	}
}

// Параметр next — это адрес, куда вернуть пользователя после входа; он должен
// доехать до шаблона, иначе после логина человек окажется на главной.
func TestPassesNextToTemplate(t *testing.T) {
	w := httptest.NewRecorder()
	New(manager(t), loginTmpl(t), logger()).Get(w, httptest.NewRequest(http.MethodGet, "/login?next=/settings", nil))
	if !strings.Contains(w.Body.String(), "next=/settings") {
		t.Fatalf("next не доехал до шаблона: %q", w.Body.String())
	}
}

// Сломанный шаблон логируется, но не роняет процесс.
func TestBrokenTemplateDoesNotPanic(t *testing.T) {
	root := template.New("root")
	if _, err := root.New("login").Parse(`{{.НетТакогоПоля.Вообще}}`); err != nil {
		t.Fatalf("parse: %v", err)
	}
	w := httptest.NewRecorder()
	New(manager(t), root, logger()).Get(w, httptest.NewRequest(http.MethodGet, "/login", nil))
	_ = w.Body.String()
}
