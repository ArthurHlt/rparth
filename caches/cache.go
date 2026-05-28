package caches

import "github.com/ArthurHlt/rparth/models"

//go:generate mockgen -destination=mocks/mock_cache.go -package=mocks . Cache

type Cache interface {
	Get(key string) (*models.CacheData, bool)
	Set(key string, data *models.CacheData)
	Close() error
	Len() int
}
