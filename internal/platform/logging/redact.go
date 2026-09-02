package logging

import (
	"log/slog"
	"strings"
)

// Mask — фиксированная замена значения, не позволяющая восстановить исходное.
const Mask = "[REDACTED]"

// deniedKeyTokens — подстроки нормализованного имени ключа, наличие которых
// означает секрет или персональные данные.
//
// Сопоставление идёт по вхождению подстроки в нормализованное имя, поэтому одно
// правило перекрывает и производные: email / user_email / OwnerEmail, token /
// access_token / refresh-token, secret / secret_key / client.secret.
var deniedKeyTokens = []string{
	"password",
	"passwd",
	"token",
	"secret",
	"authorization",
	"cookie",
	"apikey",
	"credential",
	"privatekey",
	"sessionid",
	"email",
	"webhookurl",
	// Отображаемое имя и его родственники: в логе пользователь опознаётся
	// числовым идентификатором, а не текстом, введённым человеком.
	"displayname",
	"fullname",
	"username",
}

// normalizeKey приводит имя ключа к виду, в котором сравнение не зависит от
// регистра и выбранного разделителя слов.
func normalizeKey(key string) string {
	var b strings.Builder
	b.Grow(len(key))
	for _, r := range key {
		switch r {
		case '_', '-', '.', ' ':
			continue
		}
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

// IsDeniedKey сообщает, подлежит ли значение с таким ключом редакции.
func IsDeniedKey(key string) bool {
	norm := normalizeKey(key)
	if norm == "" {
		return false
	}
	for _, token := range deniedKeyTokens {
		if strings.Contains(norm, token) {
			return true
		}
	}
	return false
}

// RedactAttr — ReplaceAttr обработчика: последний рубеж перед выводом.
//
// Основная защита — не передавать секреты и персональные данные в логгер вовсе;
// эта функция ловит то, что просочилось. Она намеренно работает по имени ключа,
// а не по значению: угадывать «похоже на токен» по содержимому означало бы и
// ложные срабатывания, и пропуски.
func RedactAttr(groups []string, a slog.Attr) slog.Attr {
	// Встроенные атрибуты записи (time, level, msg) приходят сюда с пустым
	// groups; их имена не пересекаются с денайлистом, отдельная проверка не нужна.
	if IsDeniedKey(a.Key) {
		return slog.String(a.Key, Mask)
	}
	return a
}
