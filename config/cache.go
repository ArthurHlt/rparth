package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/goccy/go-yaml"
)

type Cache struct {
	MaxSizeItem uint        `yaml:"max_size_item"`
	Lru         *LruCache   `yaml:"lru"`
	Redis       *RedisCache `yaml:"redis"`
}

func (c *Cache) UnmarshalYAML(data []byte) error {
	type plain Cache
	err := yaml.Unmarshal(data, (*plain)(c))
	if err != nil {
		return fmt.Errorf("failed to unmarshal cache: %w", err)
	}
	if c.Lru != nil && c.Redis != nil {
		return errors.New("cache: only one of lru or redis can be set")
	}
	if c.MaxSizeItem == 0 {
		// 1 MiB
		c.MaxSizeItem = 1024 * 1024
	}
	return nil
}

type LruCache struct {
	Size uint          `yaml:"size"`
	Ttl  time.Duration `yaml:"ttl"`
}

func (l *LruCache) UnmarshalYAML(data []byte) error {
	type plain LruCache
	err := yaml.Unmarshal(data, (*plain)(l))
	if err != nil {
		return fmt.Errorf("failed to unmarshal lru cache: %w", err)
	}
	if l.Size == 0 {
		l.Size = 100
	}
	if l.Ttl == 0 {
		l.Ttl = 10 * time.Minute
	}
	return nil
}

type RedisCache struct {
	URL string        `yaml:"url"`
	Ttl time.Duration `yaml:"ttl"`
}

func (r *RedisCache) UnmarshalYAML(data []byte) error {
	type plain RedisCache
	err := yaml.Unmarshal(data, (*plain)(r))
	if err != nil {
		return fmt.Errorf("failed to unmarshal redis cache: %w", err)
	}
	if r.URL == "" {
		return errors.New("redis cache: url is required")
	}
	if r.Ttl == 0 {
		r.Ttl = 10 * time.Minute
	}
	return nil
}
