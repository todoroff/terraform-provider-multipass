package multipasscli

import "time"

type cacheEntry[T any] struct {
	value   T
	expires time.Time
}

func (c *cacheEntry[T]) valid(now time.Time) bool {
	return c != nil && now.Before(c.expires)
}

func newCacheEntry[T any](value T, ttl time.Duration) *cacheEntry[T] {
	return &cacheEntry[T]{
		value:   value,
		expires: time.Now().Add(ttl),
	}
}
