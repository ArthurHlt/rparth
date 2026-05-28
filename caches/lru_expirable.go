package caches

import (
	"time"

	"github.com/ArthurHlt/rparth/models"
	"github.com/hashicorp/golang-lru/v2/expirable"
)

type EvictionCallback func(key string, value *models.CacheData)

type LRUExpirable struct {
	lruCache *expirable.LRU[string, *models.CacheData]
}

func NewLRUExpirable(size int, onEvict EvictionCallback, ttl time.Duration) *LRUExpirable {
	var evictCallback expirable.EvictCallback[string, *models.CacheData]
	if onEvict != nil {
		evictCallback = expirable.EvictCallback[string, *models.CacheData](onEvict)
	}
	lru := expirable.NewLRU[string, *models.CacheData](
		size,
		evictCallback,
		ttl,
	)
	return &LRUExpirable{lruCache: lru}
}

func (l *LRUExpirable) Get(key string) (*models.CacheData, bool) {
	return l.lruCache.Get(key)
}

func (l *LRUExpirable) Set(key string, data *models.CacheData) {
	l.lruCache.Add(key, data)
}

func (l *LRUExpirable) Delete(key string) {
	l.lruCache.Remove(key)
}
