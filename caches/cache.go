package caches

import "github.com/ArthurHlt/rparth/models"

type Cache interface {
	Get(key string) (*models.CacheData, bool)
	Set(key string, data *models.CacheData)
	Close() error
	Len() int
}
