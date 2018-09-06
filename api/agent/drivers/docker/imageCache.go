package docker

import (
	"errors"
	"sort"
	"sync"
	"time"

	d "github.com/fsouza/go-dockerclient"
	"github.com/sirupsen/logrus"
)

// Cache is an LRU cache, safe for concurrent access.
type Cache struct {
	totalSize int64
	mu        sync.Mutex
	cache     EntryByAge
	maxSize   int64
}

type Entry struct {
	lastUsed time.Time
	locked   bool
	uses     int64
	image    d.APIImages
}

func (e Entry) Score() int64 {
	age := time.Now().Sub(e.lastUsed)
	return age.Nanoseconds() / e.uses
}

type EntryByAge []Entry

func (a EntryByAge) Len() int           { return len(a) }
func (a EntryByAge) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a EntryByAge) Less(i, j int) bool { return a[i].Score() < a[j].Score() }

func NewEntry(value d.APIImages) Entry {
	return Entry{
		lastUsed: time.Now(),
		locked:   false,
		uses:     0,
		image:    value}
}

// New returns a new cache with the provided maximum items.
func NewCache(maxSize int64) *Cache {
	return &Cache{
		cache: make(EntryByAge, 0),
	}
}

func (c *Cache) Contains(value d.APIImages) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, i := range c.cache {
		if i.image.ID == value.ID {
			return true
		}
	}
	return false
}

func (c *Cache) Mark(ID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for idx, i := range c.cache {
		if i.image.ID == ID {
			c.cache[idx].lastUsed = time.Now()
			c.cache[idx].uses = c.cache[idx].uses + 1
			return nil
		}
	}

	return errors.New("Image not found in cache")
}

func (c *Cache) Remove(value d.APIImages) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for idx, i := range c.cache {
		if i.image.ID == value.ID {
			// Move the last item into the location of the item to be removed
			c.cache[idx] = c.cache[len(c.cache)-1]
			// shorten the list
			c.cache = c.cache[:len(c.cache)-1]
			return nil
		}
	}

	return errors.New("Image not found in cache")
}

func (c *Cache) Lock(ID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, i := range c.cache {
		if i.image.ID == ID {
			i.locked = true
			return nil
		}
	}
	return errors.New("Image not found in cache")
}

func (c *Cache) Locked(value d.APIImages) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, i := range c.cache {
		if i.image.ID == value.ID {
			return i.locked, nil
		}
	}
	return false, errors.New("Image not found in cache")
}

func (c *Cache) Unlock(value d.APIImages) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, i := range c.cache {
		if i.image.ID == value.ID {
			i.locked = false
		}
	}
}

// Add adds the provided key and value to the cache, evicting
// an old item if necessary.
func (c *Cache) Add(value d.APIImages) {
	c.mu.Lock()
	defer c.mu.Unlock()
	logrus.Debugf("value: %v", value)
	if c.Contains(value) {
		c.Mark(value.ID)
		return
	}
	c.cache = append(c.cache, NewEntry(value))
	c.totalSize += value.Size
}

func (c *Cache) TotalSize() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.totalSize
}

func (c *Cache) OverFilled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.totalSize < c.maxSize
}

func (c *Cache) Evictable() (ea EntryByAge) {
	for _, i := range c.cache {
		if i.locked == false {
			ea = append(ea, i)
		}
	}
	sort.Sort(ea)
	return ea
}

// Len returns the number of items in the cache.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.cache)
}
