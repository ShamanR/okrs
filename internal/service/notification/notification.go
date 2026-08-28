// Package notification is the notifications entity service: create, read, mark read,
// retention. One repository, no business rules — the fan-out lives in usecase.
package notification

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"okrs/internal/core/domain"
	"okrs/internal/store/notifications"
)

// ErrInvalidCursor is returned by List when the cursor token cannot be decoded, so
// the handler can answer 400 rather than silently starting over from the first page.
var ErrInvalidCursor = errors.New("notification: invalid cursor")

// Repo is the port this service needs. Declared consumer-side, per specs/010.
type Repo interface {
	Insert(ctx context.Context, scope domain.TenantScope, in notifications.InsertInput) (bool, error)
	InsertBatch(ctx context.Context, scope domain.TenantScope, ins []notifications.InsertInput) error
	List(ctx context.Context, scope domain.TenantScope, userID int64, f notifications.ListFilter) ([]notifications.Notification, *notifications.Cursor, error)
	UnreadCount(ctx context.Context, scope domain.TenantScope, userID int64) (int, error)
	MarkRead(ctx context.Context, scope domain.TenantScope, userID int64, ids []int64, all bool) error
	PurgeOlderThan(ctx context.Context, readDays, anyDays int) (int64, error)
}

type Service struct{ repo Repo }

func New(repo Repo) *Service { return &Service{repo: repo} }

func (s *Service) Create(ctx context.Context, scope domain.TenantScope, in notifications.InsertInput) (bool, error) {
	return s.repo.Insert(ctx, scope, in)
}

// Батчевая операция: не превращать в цикл Create — это N+1 на горячем пути fan-out.
func (s *Service) CreateBatch(ctx context.Context, scope domain.TenantScope, ins []notifications.InsertInput) error {
	return s.repo.InsertBatch(ctx, scope, ins)
}

// List returns a page of the recipient's notifications. cursor is the opaque token
// from a previous page's NextCursor (empty = first page); the caller — the HTTP
// handler — never sees the keyset position, only this string, per specs/010 (the
// activity feed is the model this mirrors). An unparsable token answers
// ErrInvalidCursor rather than silently restarting at page one: unlike the activity
// feed, the notifications API has always treated a bad cursor as a client error
// (see the handler's pre-existing 400 behaviour), so that contract is preserved
// across the move.
func (s *Service) List(ctx context.Context, scope domain.TenantScope, userID int64, f notifications.ListFilter, cursor string) ([]notifications.Notification, string, error) {
	if cursor != "" {
		c, err := decodeCursor(cursor)
		if err != nil {
			return nil, "", ErrInvalidCursor
		}
		f.Cursor = c
	}
	items, next, err := s.repo.List(ctx, scope, userID, f)
	if err != nil {
		return nil, "", err
	}
	return items, encodeCursor(next), nil
}

// Cursor encoding keeps the keyset position (created_at + id) opaque to callers
// outside this package — the same contract service/activity uses for its feed.
func encodeCursor(c *notifications.Cursor) string {
	if c == nil {
		return ""
	}
	raw := c.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(c.ID, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(s string) (*notifications.Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return nil, strconv.ErrSyntax
	}
	at, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, err
	}
	return &notifications.Cursor{CreatedAt: at, ID: id}, nil
}

func (s *Service) UnreadCount(ctx context.Context, scope domain.TenantScope, userID int64) (int, error) {
	return s.repo.UnreadCount(ctx, scope, userID)
}

func (s *Service) MarkRead(ctx context.Context, scope domain.TenantScope, userID int64, ids []int64, all bool) error {
	return s.repo.MarkRead(ctx, scope, userID, ids, all)
}

// Purge is the retention pass, run from the scheduler.
func (s *Service) Purge(ctx context.Context, readDays, anyDays int) (int64, error) {
	return s.repo.PurgeOlderThan(ctx, readDays, anyDays)
}
