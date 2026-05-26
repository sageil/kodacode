package tui

func newTranscriptRenderLRU(cap int, maxBytes int) *transcriptRenderLRU {
	if cap <= 0 {
		cap = lruDefaultCap
	}
	if maxBytes <= 0 {
		maxBytes = 8 << 20
	}
	return &transcriptRenderLRU{
		cap:      cap,
		maxBytes: maxBytes,
		entries:  make(map[string]*transcriptRenderLRUEntry, cap),
	}
}

func (c *transcriptRenderLRU) get(key string) (transcriptRender, bool) {
	if c == nil {
		return transcriptRender{}, false
	}
	entry, ok := c.entries[key]
	if !ok {
		return transcriptRender{}, false
	}
	c.moveToFront(entry)
	return entry.value, true
}

func (c *transcriptRenderLRU) put(key string, value transcriptRender) {
	if c == nil {
		return
	}
	size := transcriptRenderEntryBytes(key, value)
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
	entry := &transcriptRenderLRUEntry{
		key:   key,
		value: value,
		size:  size,
	}
	c.entries[key] = entry
	c.bytes += size
	c.pushFront(entry)
}

func (c *transcriptRenderLRU) moveToFront(entry *transcriptRenderLRUEntry) {
	if c == nil || entry == nil || c.head == entry {
		return
	}
	c.unlink(entry)
	c.pushFront(entry)
}

func (c *transcriptRenderLRU) pushFront(entry *transcriptRenderLRUEntry) {
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

func (c *transcriptRenderLRU) unlink(entry *transcriptRenderLRUEntry) {
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

func (c *transcriptRenderLRU) evict() {
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

func transcriptRenderEntryBytes(key string, rendered transcriptRender) int {
	size := len(key) + len(rendered.content)
	for ref := range rendered.toolLines {
		size += len(ref.TurnID) + len(ref.CallID) + 16
	}
	for _, line := range rendered.selectionLines {
		size += len(line.text) + 24
	}
	return size
}
