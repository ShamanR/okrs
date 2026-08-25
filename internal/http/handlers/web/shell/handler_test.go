package shell

// Оболочки и легаси-редиректы регистрируются таблицами, а не по пакету на URI.
// Из-за замыкания в цикле здесь легко получить классическую ошибку — все роуты
// начинают рендерить последний шаблон таблицы; тесты фиксируют, что каждый URI
// отдаёт свой шаблон и каждый редирект ведёт в свой адрес.

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// tmpl определяет по одному шаблону на каждое имя, встречающееся в таблицах:
// шаблон печатает своё имя, поэтому по телу ответа видно, какой из них сработал.
func tmpl(t *testing.T, names []string) *template.Template {
	t.Helper()
	root := template.New("root")
	for _, n := range names {
		if _, err := root.New(n).Parse("шаблон:" + n); err != nil {
			t.Fatalf("parse %s: %v", n, err)
		}
	}
	return root
}

func templateNames(routes []Route) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range routes {
		if !seen[r.Template] {
			seen[r.Template] = true
			out = append(out, r.Template)
		}
	}
	return out
}

func TestEachShellRendersItsOwnTemplate(t *testing.T) {
	for _, table := range []struct {
		name   string
		routes []Route
	}{
		{"Public", Public}, {"TenantAdmin", TenantAdmin}, {"System", System},
	} {
		t.Run(table.name, func(t *testing.T) {
			r := chi.NewRouter()
			New(tmpl(t, templateNames(table.routes)), func() Data { return Data{} }).RegisterShells(r, table.routes)
			for _, rt := range table.routes {
				// {teamID} и подобные заменяются конкретным значением, чтобы попасть в роут.
				uri := strings.NewReplacer("{teamID}", "1", "{periodID}", "1", "{goalID}", "1").Replace(rt.URI)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, uri, nil))
				if w.Code != http.StatusOK {
					t.Fatalf("%s: код %d", uri, w.Code)
				}
				if got := w.Body.String(); got != "шаблон:"+rt.Template {
					t.Fatalf("%s отдал %q, want шаблон %q", uri, got, rt.Template)
				}
				if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
					t.Fatalf("%s отдал Content-Type %q", uri, ct)
				}
			}
		})
	}
}

// Data вычисляется на каждый запрос, а не один раз при регистрации: флаг Dev
// приходит из окружения и может измениться между запросами в тестах и dev-режиме.
func TestDataIsEvaluatedPerRequest(t *testing.T) {
	calls := 0
	r := chi.NewRouter()
	root := template.New("root")
	_, _ = root.New("tracker-shell").Parse("{{.Dev}}")
	New(root, func() Data { calls++; return Data{Dev: calls%2 == 1} }).
		RegisterShells(r, []Route{{"/", "tracker-shell"}})

	var bodies []string
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		bodies = append(bodies, w.Body.String())
	}
	if calls != 2 {
		t.Fatalf("data() вызвана %d раз на два запроса, want 2", calls)
	}
	if bodies[0] == bodies[1] {
		t.Fatalf("оба ответа одинаковы (%q) — data() зафиксирована при регистрации", bodies[0])
	}
}

func TestEachRedirectGoesToItsOwnTarget(t *testing.T) {
	for _, table := range []struct {
		name string
		rds  []Redirect
	}{
		{"PublicRedirects", PublicRedirects}, {"MemberRedirects", MemberRedirects}, {"AdminRedirects", AdminRedirects},
	} {
		t.Run(table.name, func(t *testing.T) {
			r := chi.NewRouter()
			RegisterRedirects(r, table.rds)
			for _, rd := range table.rds {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, rd.From, nil))
				if w.Code != http.StatusFound {
					t.Fatalf("%s: код %d, want 302", rd.From, w.Code)
				}
				if got := w.Header().Get("Location"); got != rd.To {
					t.Fatalf("%s ведёт на %q, want %q", rd.From, got, rd.To)
				}
			}
		})
	}
}

// Закладки на /teamOkrs кодируют выбранную команду и период в query — редирект
// обязан их сохранить, иначе пользователь попадёт на пустую доску.
func TestKeepQueryCarriesQueryString(t *testing.T) {
	r := chi.NewRouter()
	RegisterRedirects(r, []Redirect{{From: "/legacy", To: "/canonical", KeepQuery: true}})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/legacy?team=1&period=2", nil))
	if got := w.Header().Get("Location"); got != "/canonical?team=1&period=2" {
		t.Fatalf("Location = %q, query потеряна", got)
	}
}

func TestWithoutKeepQueryTheQueryIsDropped(t *testing.T) {
	r := chi.NewRouter()
	RegisterRedirects(r, []Redirect{{From: "/legacy", To: "/canonical"}})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/legacy?team=1", nil))
	if got := w.Header().Get("Location"); got != "/canonical" {
		t.Fatalf("Location = %q, want /canonical", got)
	}
}
