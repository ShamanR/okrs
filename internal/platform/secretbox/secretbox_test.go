package secretbox_test

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"okrs/internal/platform/secretbox"
)

func newKey(t *testing.T) string {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return base64.StdEncoding.EncodeToString(k)
}

func TestSealOpenRoundTrip(t *testing.T) {
	b, err := secretbox.New(newKey(t))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	const secret = "xoxb-очень-секретный-токен-4821"
	ct, err := b.Seal(secret)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if strings.Contains(string(ct), "секретный") {
		t.Fatal("плейнтекст виден в шифротексте")
	}
	got, err := b.Open(ct)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if got != secret {
		t.Fatalf("round-trip: got %q", got)
	}
}

// Каждое шифрование обязано давать новый nonce, иначе одинаковые секреты
// разных тенантов дадут одинаковый шифротекст и станут сравнимы по БД.
func TestSealIsNonDeterministic(t *testing.T) {
	b, _ := secretbox.New(newKey(t))
	a, _ := b.Seal("одно и то же")
	c, _ := b.Seal("одно и то же")
	if string(a) == string(c) {
		t.Fatal("два шифрования одного значения совпали — nonce не случаен")
	}
}

// Испорченный шифротекст обязан давать ошибку, а не тихо возвращать мусор:
// GCM аутентифицирует данные, и эта проверка фиксирует, что мы её не потеряли.
func TestOpenRejectsTamperedCiphertext(t *testing.T) {
	b, _ := secretbox.New(newKey(t))
	ct, _ := b.Seal("секрет")
	ct[len(ct)-1] ^= 0xFF
	if _, err := b.Open(ct); err == nil {
		t.Fatal("порча шифротекста должна давать ошибку")
	}
}

// Чужой ключ не должен расшифровывать: иначе ротация ключа была бы бессмысленна.
func TestOpenWithWrongKeyFails(t *testing.T) {
	b1, _ := secretbox.New(newKey(t))
	b2, _ := secretbox.New(newKey(t))
	ct, _ := b1.Seal("секрет")
	if _, err := b2.Open(ct); err == nil {
		t.Fatal("другой ключ не должен расшифровывать")
	}
}

// Отсутствие ключа — не паника и не молчаливая работа без шифрования, а явная
// ошибка: коробочная установка без ключа просто не получает каналов с секретом.
func TestNewWithoutKeyReturnsErrNoKey(t *testing.T) {
	if _, err := secretbox.New(""); !errors.Is(err, secretbox.ErrNoKey) {
		t.Fatalf("got %v, want ErrNoKey", err)
	}
}

func TestNewRejectsWrongKeyLength(t *testing.T) {
	short := base64.StdEncoding.EncodeToString([]byte("слишком короткий"))
	if _, err := secretbox.New(short); err == nil || errors.Is(err, secretbox.ErrNoKey) {
		t.Fatalf("короткий ключ должен давать ошибку длины, got %v", err)
	}
}

// Маска показывается в UI вместо секрета. Она обязана быть узнаваемой, но не
// восстановимой: четыре последних символа — компромисс, принятый в спеке §7.4.
func TestHintShowsOnlyTail(t *testing.T) {
	if got := secretbox.Hint("xoxb-abcdef4821"); got != "••••4821" {
		t.Fatalf("got %q, want ••••4821", got)
	}
	// Короткий секрет не должен раскрываться целиком.
	if got := secretbox.Hint("abc"); strings.Contains(got, "abc") {
		t.Fatalf("короткий секрет раскрыт: %q", got)
	}
	if got := secretbox.Hint(""); got != "" {
		t.Fatalf("пустой секрет должен давать пустую маску, got %q", got)
	}
}
