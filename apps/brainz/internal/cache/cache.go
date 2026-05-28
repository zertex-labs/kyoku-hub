package cache

import (
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"go.uber.org/zap"

	"github.com/zertex-labs/kyoku-hub/apps/brainz/internal/config"
)

type Cache struct {
	store  *ristretto.Cache[string, []byte]
	tiers  []config.CacheTier
	logger *zap.Logger
}

func New(cfg *config.Config, logger *zap.Logger) (*Cache, error) {
	store, err := ristretto.NewCache(&ristretto.Config[string, []byte]{
		NumCounters: 1e6,
		MaxCost:     cfg.CacheMaxSizeMB * 1024 * 1024,
		BufferItems: 64,
	})
	if err != nil {
		return nil, err
	}

	return &Cache{
		store:  store,
		tiers:  cfg.CacheTTLTiers,
		logger: logger,
	}, nil
}

func (c *Cache) Get(key string) ([]byte, bool) {
	val, found := c.store.Get(key)
	return val, found
}

// Set stores a value with TTL based on hit count.
// For now, uses flat TTL (lowest tier). Adaptive TTL will be added later.
func (c *Cache) Set(key string, value []byte) {
	ttl := c.tiers[len(c.tiers)-1].TTL // default to lowest tier
	c.store.SetWithTTL(key, value, int64(len(value)), ttl)
}

func (c *Cache) SetWithHits(key string, value []byte, hits int) {
	ttl := c.resolveTTL(hits)
	c.store.SetWithTTL(key, value, int64(len(value)), ttl)
}

func (c *Cache) Delete(key string) {
	c.store.Del(key)
}

func (c *Cache) resolveTTL(hits int) time.Duration {
	for _, tier := range c.tiers {
		if hits >= tier.MinHits {
			return tier.TTL
		}
	}
	return 10 * time.Minute
}

func (c *Cache) Close() {
	c.store.Close()
}
