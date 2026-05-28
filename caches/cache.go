package caches

import "github.com/ArthurHlt/rparth/models"

type Cache interface {
	Get(key string) (*models.CacheData, bool)
	Set(key string, data *models.CacheData)
	Contains(key string) bool
	Close() error
}
