package docker

import (
	"container/list"
	"sync"

	d "github.com/fsouza/go-dockerclient"
)

type ImageEvictor func(d.APIImages) error

// Cache is an LRU cache, safe for concurrent access.
type Cache struct {
	totalSize int64
	mu        sync.Mutex
	ll        *list.List
	cache     map[string]*list.Element
	maxSize   int64
	onEvict   ImageEvictor
}

// *entry is the type stored in each *list.Element.
type entry struct {
	key   string
	value d.APIImages
}

// New returns a new cache with the provided maximum items.
func NewLRU(maxSize int64, onEvict ImageEvictor) *Cache {
	return &Cache{
		ll:      list.New(),
		cache:   make(map[string]*list.Element),
		onEvict: onEvict,
	}
}

// Add adds the provided key and value to the cache, evicting
// an old item if necessary.
func (c *Cache) Add(key string, value d.APIImages) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Already in cache?
	if ee, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ee)
		c.totalSize = (c.totalSize - ee.Value.(d.APIImages).Size) + value.Size
		ee.Value.(*entry).value = value
		return
	}

	// Add to cache if not present
	ele := c.ll.PushFront(&entry{key, value})
	c.cache[key] = ele
	c.totalSize += value.Size

	for c.TotalSize() > c.maxSize {
		c.RemoveOldest()
	}
}

func (c *Cache) TotalSize() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.totalSize
}

// Get fetches the key's value from the cache.
// The ok result will be true if the item was found.
func (c *Cache) Get(key string) (value d.APIImages, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ele, hit := c.cache[key]; hit {
		c.ll.MoveToFront(ele)
		return ele.Value.(*entry).value, true
	}
	return
}

// RemoveOldest removes the oldest item in the cache and returns its key and value.
// If the cache is empty, the empty string and nil are returned.
func (c *Cache) RemoveOldest() (key string, value d.APIImages) {
	c.mu.Lock()
	k, v := c.removeOldest()
	c.mu.Unlock()
	c.onEvict(v)
	return k, v
}

// note: must hold c.mu
func (c *Cache) removeOldest() (key string, value d.APIImages) {
	ele := c.ll.Back()
	if ele == nil {
		return
	}
	c.ll.Remove(ele)
	ent := ele.Value.(*entry)
	c.totalSize -= value.Size
	delete(c.cache, ent.key)
	return ent.key, ent.value

}

// Len returns the number of items in the cache.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}
