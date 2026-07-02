package auth

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"okrs/internal/domain"
	"okrs/internal/store/users"
)

// fakeAuthStore implements authStorage + userGranter for Login bootstrap tests.
type fakeAuthStore struct {
	nextID       int64
	byKey        map[string]*domain.User
	systemAdmins map[int64]bool
}

func newFakeAuthStore() *fakeAuthStore {
	return &fakeAuthStore{byKey: map[string]*domain.User{}, systemAdmins: map[int64]bool{}}
}

func (f *fakeAuthStore) UpsertUser(_ context.Context, in users.UpsertUserInput) (*domain.User, error) {
	u, ok := f.byKey[in.ProviderSubjectKey]
	if !ok {
		f.nextID++
		u = &domain.User{ID: f.nextID, ProviderSubjectKey: in.ProviderSubjectKey, Provider: in.Provider, Subject: in.Subject}
		f.byKey[in.ProviderSubjectKey] = u
	}
	u.DisplayName = in.DisplayName
	u.Email = in.Email
	u.IsSystemAdmin = f.systemAdmins[u.ID]
	return u, nil
}

func (f *fakeAuthStore) GetUser(context.Context, int64) (*domain.User, error) { return nil, nil }
func (f *fakeAuthStore) CreateSession(_ context.Context, id string, userID int64, provider string, _ time.Duration, _, _ string) (*domain.AuthSession, error) {
	return &domain.AuthSession{ID: id, UserID: userID}, nil
}
func (f *fakeAuthStore) GetSession(context.Context, string) (*domain.AuthSession, error) {
	return nil, nil
}
func (f *fakeAuthStore) TouchSession(context.Context, string) error  { return nil }
func (f *fakeAuthStore) DeleteSession(context.Context, string) error { return nil }
func (f *fakeAuthStore) GetSetting(context.Context, string) (json.RawMessage, error) {
	return nil, nil
}
func (f *fakeAuthStore) GetTenantSetting(context.Context, domain.TenantScope, string) (json.RawMessage, error) {
	return nil, nil
}
func (f *fakeAuthStore) AnySystemAdmin(context.Context) (bool, error) {
	for _, v := range f.systemAdmins {
		if v {
			return true, nil
		}
	}
	return false, nil
}
func (f *fakeAuthStore) SetSystemAdmin(_ context.Context, userID int64, v bool) error {
	f.systemAdmins[userID] = v
	return nil
}
func TestLoginBootstrapsFirstSystemAdmin(t *testing.T) {
	store := newFakeAuthStore()
	cfg := DefaultConfig()
	cfg.BootstrapSystemAdmin = "github:42"
	mgr, err := NewManager(cfg, store)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	ctx := context.Background()

	// The matching identity is promoted.
	u, _, err := mgr.Login(ctx, &Identity{Provider: "github", Subject: "42", DisplayName: "Root"}, "ua", "ip")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !u.IsSystemAdmin {
		t.Fatal("bootstrap identity should be promoted to system admin")
	}

	// A second, different identity is not promoted (a system admin already exists).
	u2, _, err := mgr.Login(ctx, &Identity{Provider: "github", Subject: "99", DisplayName: "Other"}, "ua", "ip")
	if err != nil {
		t.Fatalf("login 2: %v", err)
	}
	if u2.IsSystemAdmin {
		t.Fatal("second identity must not be promoted once a system admin exists")
	}
}
