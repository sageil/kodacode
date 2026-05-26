package tui

// lruCache stores string values with bounded size and evicts the least-recently
// used entry when full.
type lruCache struct {
	cap      int
	maxBytes int
	bytes    int
	entries  map[string]*lruEntry
	head     *lruEntry
	tail     *lruEntry
}

type lruEntry struct {
	key        string
	value      string
	size       int
	prev, next *lruEntry
}

const lruDefaultCap = 64
const lruDefaultMaxBytes = 4 << 20

func newLRUCache(cap int, maxBytes int) *lruCache {
	if cap <= 0 {
		cap = lruDefaultCap
	}
	if maxBytes <= 0 {
		maxBytes = lruDefaultMaxBytes
	}
	return &lruCache{
		cap:      cap,
		maxBytes: maxBytes,
		entries:  make(map[string]*lruEntry, cap),
	}
}

func (c *lruCache) get(key string) (string, bool) {
	if c == nil {
		return "", false
	}
	entry, ok := c.entries[key]
	if !ok {
		return "", false
	}
	c.moveToFront(entry)
	return entry.value, true
}

func (c *lruCache) put(key, value string) {
	if c == nil {
		return
	}
	size := len(key) + len(value)
	if size > c.maxBytes {
		return
	}
	if entry, ok := c.entries[key]; ok {
		c.bytes += size - entry.size
		entry.value = value
		entry.size = size
		c.moveToFront(entry)
		for len(c.entries) > c.cap || c.bytes > c.maxBytes {
			c.evict()
		}
		return
	}
	for len(c.entries) >= c.cap || c.bytes+size > c.maxBytes {
		c.evict()
	}
	entry := &lruEntry{key: key, value: value, size: size}
	c.entries[key] = entry
	c.bytes += size
	c.pushFront(entry)
}

func (c *lruCache) moveToFront(entry *lruEntry) {
	if c == nil || entry == nil || c.head == entry {
		return
	}
	c.unlink(entry)
	c.pushFront(entry)
}

func (c *lruCache) pushFront(entry *lruEntry) {
	if c == nil || entry == nil {
		return
	}
	entry.prev = nil
	entry.next = c.head
	if c.head != nil {
		c.head.prev = entry
	}
	c.head = entry
	if c.tail == nil {
		c.tail = entry
	}
}

func (c *lruCache) unlink(entry *lruEntry) {
	if c == nil || entry == nil {
		return
	}
	if entry.prev != nil {
		entry.prev.next = entry.next
	} else {
		c.head = entry.next
	}
	if entry.next != nil {
		entry.next.prev = entry.prev
	} else {
		c.tail = entry.prev
	}
}

func (c *lruCache) evict() {
	if c == nil || c.tail == nil {
		return
	}
	entry := c.tail
	c.unlink(entry)
	delete(c.entries, entry.key)
	c.bytes -= entry.size
	if c.bytes < 0 {
		c.bytes = 0
	}
}
