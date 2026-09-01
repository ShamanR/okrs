package notificationchannels_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"okrs/internal/http/handlers/api/v1/system/notificationchannels"
	"okrs/internal/http/handlers/handlertest"
	"okrs/notifychannel"
	"okrs/notifychannel/mattermost"
)

type fakeSvc struct {
	ds []notifychannel.Descriptor
}

func (f fakeSvc) Descriptors() []notifychannel.Descriptor { return f.ds }

// Панель выдаёт каналы пространствам, поэтому ей нужен список того, что есть в
// сборке, — вместе с готовым ключом entitlement, чтобы не собирать его в JS.
func TestListReturnsBuildChannelsWithEntitlementKeys(t *testing.T) {
	h := notificationchannels.New(fakeSvc{ds: []notifychannel.Descriptor{
		mattermost.Channel().Descriptor,
	}})

	rec := handlertest.Do(h.List, http.MethodGet, "/api/v1/system/notification-channels", "")
	handlertest.Status(t, rec, http.StatusOK)

	var got struct {
		Channels []struct {
			Name           string `json:"name"`
			Title          string `json:"title"`
			EntitlementKey string `json:"entitlement_key"`
		} `json:"channels"`
	}
	handlertest.DecodeJSON(t, rec, &got)
	if len(got.Channels) != 1 {
		t.Fatalf("каналы: %+v", got.Channels)
	}
	c := got.Channels[0]
	if c.Name != "mattermost" || c.Title != "Mattermost" {
		t.Fatalf("канал: %+v", c)
	}
	// Голый ключ, без префикса: SetEntitlements добавит "entitlement." сам, и
	// панель, отправив полный ключ, получила бы "entitlement.entitlement.…".
	if c.EntitlementKey != "notifications.mattermost" {
		t.Fatalf("ключ entitlement: %q", c.EntitlementKey)
	}
}

// Сборка без каналов отвечает пустым массивом, а не null: JS итерирует поле.
func TestListWithNoChannelsReturnsEmptyArray(t *testing.T) {
	h := notificationchannels.New(fakeSvc{})
	rec := handlertest.Do(h.List, http.MethodGet, "/api/v1/system/notification-channels", "")
	handlertest.Status(t, rec, http.StatusOK)

	var got struct {
		Channels []json.RawMessage `json:"channels"`
	}
	handlertest.DecodeJSON(t, rec, &got)
	if got.Channels == nil {
		t.Fatalf("ожидался пустой массив, а не null: %s", handlertest.Body(rec))
	}
	if len(got.Channels) != 0 {
		t.Fatalf("каналы: %s", handlertest.Body(rec))
	}
}
