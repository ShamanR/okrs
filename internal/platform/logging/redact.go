package logging

import (
	"log/slog"
	"regexp"
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

// emailPattern ловит адрес электронной почты внутри произвольного текста,
// включая percent-encoded форму (%40 вместо @), в которой адрес попадает в URL.
var emailPattern = regexp.MustCompile(`[A-Za-z0-9._+\-]+(?:@|%40)[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)

// maskEmails заменяет адреса почты внутри значения на маску.
//
// Это единственное правило, работающее по содержимому, а не по имени ключа, и оно
// обязательно: адрес попадает в лог не отдельным атрибутом, а внутри текста
// ошибки внешнего канала: Mattermost адресует получателя путём
// /api/v4/users/email/<escaped email>, и этот путь оборачивается в ошибку. Редакция
// по имени ключа такое не видит, а спецификация запрещает адрес в логах безоговорочно.
//
// От токенов и паролей это отличается тем, что адрес почты — чётко описанный
// шаблон, а «похоже на токен» — догадка; поэтому по содержимому мы ищем только
// адреса.
func maskEmails(s string) string {
	// Дешёвая проверка перед регуляркой: подавляющее большинство значений
	// ни @, ни %40 не содержит, и для них регулярка не запускается вовсе.
	if !strings.Contains(s, "@") && !strings.Contains(s, "%40") {
		return s
	}
	return emailPattern.ReplaceAllString(s, Mask)
}

// RedactAttr — ReplaceAttr обработчика: последний рубеж перед выводом.
//
// Основная защита — не передавать секреты и персональные данные в логгер вовсе;
// эта функция ловит то, что просочилось, в два рубежа: по имени ключа (секреты
// и очевидные PII-поля) и по содержимому — только для адресов почты, которые
// приходят внутри чужого текста.
func RedactAttr(groups []string, a slog.Attr) slog.Attr {
	// Встроенные атрибуты записи (time, level, msg) приходят сюда с пустым
	// groups; их имена не пересекаются с денайлистом, отдельная проверка не нужна.
	if IsDeniedKey(a.Key) {
		return slog.String(a.Key, Mask)
	}
	if a.Value.Kind() == slog.KindString {
		if masked := maskEmails(a.Value.String()); masked != a.Value.String() {
			return slog.String(a.Key, masked)
		}
	}
	return a
}
