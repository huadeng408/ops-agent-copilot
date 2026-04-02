package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type memoryCacheEntry struct {
	value     string
	expiresAt time.Time
}

type CacheService struct {
	client        *redis.Client
	mu            sync.RWMutex
	memoryStore   map[string]memoryCacheEntry
	disabledUntil time.Time
}

func NewCacheService(cacheURL string) *CacheService {
	service := &CacheService{
		memoryStore: make(map[string]memoryCacheEntry),
	}
	if strings.TrimSpace(cacheURL) == "" {
		return service
	}
	opt, err := redis.ParseURL(cacheURL)
	if err != nil {
		return service
	}
	opt.ReadTimeout = 200 * time.Millisecond
	opt.WriteTimeout = 200 * time.Millisecond
	opt.DialTimeout = 200 * time.Millisecond
	service.client = redis.NewClient(opt)
	return service
}

func (c *CacheService) GetJSON(ctx context.Context, key string, dest any) bool {
	payload, ok := c.get(ctx, key)
	if !ok {
		return false
	}
	if err := json.Unmarshal([]byte(payload), dest); err != nil {
		return false
	}
	return true
}

func (c *CacheService) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) {
	payload, err := json.Marshal(value)
	if err != nil {
		return
	}
	c.set(ctx, key, string(payload), ttl)
}

func (c *CacheService) Delete(ctx context.Context, key string) {
	c.mu.Lock()
	delete(c.memoryStore, key)
	c.mu.Unlock()

	if !c.redisEnabled() {
		return
	}
	if err := c.client.Del(ctx, key).Err(); err != nil {
		c.disableRedis()
	}
}

func (c *CacheService) get(ctx context.Context, key string) (string, bool) {
	if c.redisEnabled() {
		value, err := c.client.Get(ctx, key).Result()
		if err == nil {
			return value, true
		}
		if err != redis.Nil {
			c.disableRedis()
		}
	}

	c.mu.RLock()
	entry, ok := c.memoryStore[key]
	c.mu.RUnlock()
	if !ok {
		return "", false
	}
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.memoryStore, key)
		c.mu.Unlock()
		return "", false
	}
	return entry.value, true
}

func (c *CacheService) set(ctx context.Context, key string, value string, ttl time.Duration) {
	if c.redisEnabled() {
		if err := c.client.Set(ctx, key, value, ttl).Err(); err == nil {
			return
		}
		c.disableRedis()
	}

	entry := memoryCacheEntry{value: value}
	if ttl > 0 {
		entry.expiresAt = time.Now().Add(ttl)
	}
	c.mu.Lock()
	c.memoryStore[key] = entry
	c.mu.Unlock()
}

func (c *CacheService) redisEnabled() bool {
	if c.client == nil {
		return false
	}
	c.mu.RLock()
	disabledUntil := c.disabledUntil
	c.mu.RUnlock()
	return time.Now().After(disabledUntil)
}

func (c *CacheService) disableRedis() {
	c.mu.Lock()
	c.disabledUntil = time.Now().Add(time.Minute)
	c.mu.Unlock()
}
