package tui

// lruCache is a simple bounded cache that evicts the oldest entry when full.
// It uses a doubly-linked list for O(1) eviction and a map for O(1) lookup.
type lruCache struct {
	cap     int
	entries map[string]*lruEntry
	head    *lruEntry // most recent
	tail    *lruEntry // least recent
}

type lruEntry struct {
	key        string
	value      string
	prev, next *lruEntry
}

const lruDefaultCap = 64

func newLRUCache(cap int) *lruCache {
	return &lruCache{cap: cap, entries: make(map[string]*lruEntry, cap)}
}

func (c *lruCache) get(key string) (string, bool) {
	e, ok := c.entries[key]
	if !ok {
		return "", false
	}
	c.moveToFront(e)
	return e.value, true
}

func (c *lruCache) put(key, value string) {
	if e, ok := c.entries[key]; ok {
		e.value = value
		c.moveToFront(e)
		return
	}
	if len(c.entries) >= c.cap {
		c.evict()
	}
	e := &lruEntry{key: key, value: value}
	c.entries[key] = e
	c.pushFront(e)
}

func (c *lruCache) moveToFront(e *lruEntry) {
	if c.head == e {
		return
	}
	c.unlink(e)
	c.pushFront(e)
}

func (c *lruCache) pushFront(e *lruEntry) {
	e.prev = nil
	e.next = c.head
	if c.head != nil {
		c.head.prev = e
	}
	c.head = e
	if c.tail == nil {
		c.tail = e
	}
}

func (c *lruCache) unlink(e *lruEntry) {
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		c.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		c.tail = e.prev
	}
}

func (c *lruCache) evict() {
	if c.tail == nil {
		return
	}
	e := c.tail
	c.unlink(e)
	delete(c.entries, e.key)
}
