package notificationprefs_test

import (
	"reflect"
	"testing"

	"okrs/internal/store/notificationprefs"
)

// Порядок каталога — порядок экрана настроек: типы одной категории идут подряд,
// «Цели» раньше «Системных».
func TestCatalogOrderGroupsCategories(t *testing.T) {
	want := []string{
		notificationprefs.TypeGoalComment,
		notificationprefs.TypeMyCommentResolved,
		notificationprefs.TypeGoalChanged,
		notificationprefs.TypeKRProgress,
		notificationprefs.TypeAccessRequested,
	}
	if !reflect.DeepEqual(notificationprefs.AllTypes, want) {
		t.Fatalf("AllTypes = %v, want %v", notificationprefs.AllTypes, want)
	}
	seen := map[string]bool{}
	prev := ""
	for _, typ := range notificationprefs.AllTypes {
		c := notificationprefs.CategoryOf(typ)
		if c != prev && seen[c] {
			t.Fatalf("категория %q разорвана в каталоге", c)
		}
		seen[c] = true
		prev = c
	}
}

func TestCatalogAttributes(t *testing.T) {
	cases := []struct {
		typ            string
		category       string
		audience       string
		addressed      bool
		defaultEnabled bool
	}{
		{notificationprefs.TypeGoalComment, notificationprefs.CategoryGoals, notificationprefs.AudienceTeamTree, false, true},
		{notificationprefs.TypeMyCommentResolved, notificationprefs.CategoryGoals, notificationprefs.AudienceAddressee, true, true},
		{notificationprefs.TypeGoalChanged, notificationprefs.CategoryGoals, notificationprefs.AudienceTeamTree, false, true},
		{notificationprefs.TypeKRProgress, notificationprefs.CategoryGoals, notificationprefs.AudienceTeamTree, false, true},
		// Системный тип выключен по умолчанию: включение — явный выбор администратора,
		// иначе выкатка разослала бы заявки всем администраторам разом.
		{notificationprefs.TypeAccessRequested, notificationprefs.CategorySystem, notificationprefs.AudienceTenantAdmins, true, false},
	}
	for _, c := range cases {
		if got := notificationprefs.CategoryOf(c.typ); got != c.category {
			t.Errorf("%s: категория %q, want %q", c.typ, got, c.category)
		}
		if got := notificationprefs.AudienceOf(c.typ); got != c.audience {
			t.Errorf("%s: получатели %q, want %q", c.typ, got, c.audience)
		}
		if got := notificationprefs.IsAddressed(c.typ); got != c.addressed {
			t.Errorf("%s: адресный = %v, want %v", c.typ, got, c.addressed)
		}
		if got := notificationprefs.DefaultEnabled(c.typ); got != c.defaultEnabled {
			t.Errorf("%s: включён по умолчанию = %v, want %v", c.typ, got, c.defaultEnabled)
		}
	}
}

func TestTypesForRole(t *testing.T) {
	member := notificationprefs.TypesFor(false)
	if len(member) != 4 {
		t.Fatalf("участнику доступно %d типов, want 4: %v", len(member), member)
	}
	for _, typ := range member {
		if typ == notificationprefs.TypeAccessRequested {
			t.Fatal("заявка на доступ не должна быть доступна обычному участнику")
		}
	}
	if admin := notificationprefs.TypesFor(true); !reflect.DeepEqual(admin, notificationprefs.AllTypes) {
		t.Fatalf("администратору доступны %v, want все %v", admin, notificationprefs.AllTypes)
	}
}

func TestUnknownTypeHasNoAttributes(t *testing.T) {
	if notificationprefs.IsAddressed("bogus") || notificationprefs.DefaultEnabled("bogus") ||
		notificationprefs.CategoryOf("bogus") != "" || notificationprefs.AudienceOf("bogus") != "" {
		t.Fatal("у неизвестного типа не должно быть атрибутов каталога")
	}
}
