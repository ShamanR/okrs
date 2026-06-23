package settings

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

const defaultSystemSettingsCacheTTL = 5 * time.Minute

type listAllBackend interface {
	ListAll(ctx context.Context) (map[string]json.RawMessage, error)
}

// SystemSettingsCache holds the (tiny) global system_settings snapshot.
type SystemSettingsCache struct {
	backend  listAllBackend
	ttl      time.Duration
	mu       sync.RWMutex
	snapshot map[string]json.RawMessage
	cachedAt time.Time
	loaded   bool
}

func NewSystemSettingsCache(repo *SettingsRepository) *SystemSettingsCache {
	return newSystemSettingsCacheWithBackend(repo, defaultSystemSettingsCacheTTL)
}

func newSystemSettingsCacheWithBackend(b listAllBackend, ttl time.Duration) *SystemSettingsCache {
	return &SystemSettingsCache{backend: b, ttl: ttl}
}

func (c *SystemSettingsCache) GetAll(ctx context.Context) (map[string]json.RawMessage, error) {
	c.mu.RLock()
	if c.loaded && time.Since(c.cachedAt) < c.ttl {
		snap := c.snapshot
		c.mu.RUnlock()
		return snap, nil
	}
	c.mu.RUnlock()
	snap, err := c.backend.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.snapshot, c.cachedAt, c.loaded = snap, time.Now(), true
	c.mu.Unlock()
	return snap, nil
}

func (c *SystemSettingsCache) Get(ctx context.Context, key string) (json.RawMessage, error) {
	snap, err := c.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	return snap[key], nil
}

func (c *SystemSettingsCache) Invalidate() {
	c.mu.Lock()
	c.loaded = false
	c.mu.Unlock()
}
