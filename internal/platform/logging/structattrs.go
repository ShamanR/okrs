package logging

import (
	"log/slog"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

// LogTag помечает поля, которые нельзя решить по типу.
//
//	log:"safe"  — строковое поле, безопасное для лога: перечислимое значение или
//	              служебная строка, а не введённый пользователем текст.
//	log:"keys"  — для map: в лог идут только ключи, но не значения.
//	log:"-"     — не логировать, даже если тип разрешён.
const LogTag = "log"

// StructAttrs извлекает из структуры поля, безопасные для лога.
//
// Отбор идёт по ТИПУ поля, а не по имени: числа, флаги, времена и их указатели
// и срезы попадают в лог, строки — никогда, если автор не пометил поле тегом
// log:"safe".
//
// Правило выбрано именно таким, потому что оно даёт структурную, а не
// дисциплинарную гарантию. Доменные события несут пользовательский текст —
// названия целей, тексты комментариев, заметки чек-инов, — и подход «логируем всё,
// кроме перечисленного» рано или поздно пропустил бы новое текстовое поле.
// Здесь забывчивость автора нового события приводит к недологированию, а не к
// утечке комментария в систему логов: направление отказа правильное.
//
// Вложенные структуры (в том числе встроенная Meta доменного события) не
// раскрываются: их поля обрабатывает вызывающий код, который знает их смысл.
func StructAttrs(v any) []slog.Attr {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}

	attrs := make([]slog.Attr, 0, rv.NumField())
	for _, d := range descriptorsFor(rv.Type()) {
		if a, ok := d.attr(rv.Field(d.index)); ok {
			attrs = append(attrs, a)
		}
	}
	return attrs
}

type fieldMode int

const (
	modeValue fieldMode = iota // значение как есть
	modeKeys                   // только ключи map: какие поля менялись, но не на что
)

type fieldDesc struct {
	key   string
	index int
	mode  fieldMode
}

func (d fieldDesc) attr(fv reflect.Value) (slog.Attr, bool) {
	if d.mode == modeKeys {
		if fv.Len() == 0 {
			return slog.Attr{}, false
		}
		keys := make([]string, 0, fv.Len())
		for _, k := range fv.MapKeys() {
			keys = append(keys, k.String())
		}
		// Порядок обхода map в Go случаен; без сортировки одно и то же изменение
		// давало бы разные строки и не искалось бы по точному совпадению.
		sort.Strings(keys)
		return slog.Any(d.key, keys), true
	}
	if fv.Kind() == reflect.Pointer {
		if fv.IsNil() {
			return slog.Attr{}, false
		}
		fv = fv.Elem()
	}
	return slog.Any(d.key, fv.Interface()), true
}

// descCache хранит разбор по reflect.Type: рефлексия выполняется один раз на тип,
// а не на каждое опубликованное событие.
var descCache sync.Map // reflect.Type -> []fieldDesc

func descriptorsFor(t reflect.Type) []fieldDesc {
	if cached, ok := descCache.Load(t); ok {
		return cached.([]fieldDesc)
	}
	descs := buildDescriptors(t)
	actual, _ := descCache.LoadOrStore(t, descs)
	return actual.([]fieldDesc)
}

func buildDescriptors(t reflect.Type) []fieldDesc {
	var descs []fieldDesc
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get(LogTag)
		if tag == "-" {
			continue
		}
		mode, ok := modeFor(f.Type, tag)
		if !ok {
			continue
		}
		descs = append(descs, fieldDesc{key: snakeCase(f.Name), index: i, mode: mode})
	}
	return descs
}

// modeFor решает, попадает ли поле такого типа в лог.
func modeFor(t reflect.Type, tag string) (fieldMode, bool) {
	if tag == "keys" {
		return modeKeys, t.Kind() == reflect.Map && t.Key().Kind() == reflect.String
	}
	safe := tag == "safe"

	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == reflect.TypeOf(time.Time{}) {
		return modeValue, true
	}
	switch t.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return modeValue, true
	case reflect.String:
		// Только по явной пометке: перечислимое значение или служебная строка.
		return modeValue, safe
	case reflect.Slice, reflect.Array:
		mode, ok := modeFor(t.Elem(), tag)
		return mode, ok
	default:
		// Структуры, map без тега, интерфейсы, функции: смысл их содержимого
		// известен только вызывающему коду.
		return modeValue, false
	}
}

// knownAcronyms разбивает слитные прописные фрагменты имён полей. Без них KRID
// превратилось бы в krid, а не в kr_id.
var knownAcronyms = []string{"OKR", "KR", "ID", "URL", "API", "HTTP", "UI", "DB"}

// snakeCase переводит имя поля Go в ключ лог-записи: GoalID → goal_id,
// SourceTeamID → source_team_id, KRTitle → kr_title, SharedWithTeamIDs →
// shared_with_team_ids.
func snakeCase(name string) string {
	var out []string
	for _, token := range splitTokens(name) {
		out = append(out, strings.ToLower(token))
	}
	return strings.Join(out, "_")
}

func splitTokens(s string) []string {
	var tokens []string
	for i := 0; i < len(s); {
		if !isUpper(s[i]) {
			j := i
			for j < len(s) && !isUpper(s[j]) {
				j++
			}
			tokens = append(tokens, s[i:j])
			i = j
			continue
		}

		// Прописной фрагмент: собираем весь прогон заглавных.
		j := i + 1
		for j < len(s) && isUpper(s[j]) {
			j++
		}
		plural := j < len(s) && s[j] == 's'
		if j-i > 1 && j < len(s) && isLower(s[j]) && !plural {
			// Последняя заглавная начинает следующее слово: KRTitle → KR|Title.
			j--
		}
		run := s[i:j]
		if plural {
			j++
		}

		if acr := splitAcronyms(run); len(acr) > 0 {
			if plural {
				acr[len(acr)-1] += "s"
			}
			tokens = append(tokens, acr...)
			i = j
			continue
		}

		// Не прогон, а начало обычного слова: добираем строчные.
		for j < len(s) && !isUpper(s[j]) {
			j++
		}
		tokens = append(tokens, s[i:j])
		i = j
	}
	return tokens
}

// splitAcronyms режет прогон заглавных на известные аббревиатуры. Возвращает nil,
// если прогон — это просто первая буква обычного слова.
func splitAcronyms(run string) []string {
	if len(run) < 2 {
		return nil
	}
	var out []string
	for i := 0; i < len(run); {
		matched := ""
		for _, a := range knownAcronyms {
			if len(a) > len(matched) && strings.HasPrefix(run[i:], a) {
				matched = a
			}
		}
		if matched == "" {
			out = append(out, run[i:])
			break
		}
		out = append(out, matched)
		i += len(matched)
	}
	return out
}

func isUpper(c byte) bool { return c >= 'A' && c <= 'Z' }
func isLower(c byte) bool { return c >= 'a' && c <= 'z' }
