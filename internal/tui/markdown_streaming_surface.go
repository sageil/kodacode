package tui

import "strings"

type streamingMarkdownSurface struct {
	contextKey         string
	stablePrefix       string
	stablePrefixRender string
}

func (s *streamingMarkdownSurface) Reset() {
	s.contextKey = ""
	s.stablePrefix = ""
	s.stablePrefixRender = ""
}

func (s *streamingMarkdownSurface) Render(content, contextKey string, render func(string) string) string {
	full := func() string {
		return render(content)
	}

	if contextKey != s.contextKey || !strings.HasPrefix(content, s.stablePrefix) {
		s.Reset()
		s.contextKey = contextKey
		out := full()
		s.tryAdvanceFromEmpty(content, contextKey, render)
		return out
	}

	boundary := findSafeMarkdownBoundary(content)
	if boundary < 0 {
		return full()
	}

	if boundary <= len(s.stablePrefix) {
		trail := content[len(s.stablePrefix):]
		return glueStreamingMarkdownSurface(s.stablePrefixRender, s.renderTrailing(trail, render))
	}

	newChunk := content[len(s.stablePrefix):boundary]
	newChunkRender := s.renderTrailing(newChunk, render)
	s.stablePrefixRender = glueStreamingMarkdownSurface(s.stablePrefixRender, newChunkRender)
	s.stablePrefix = content[:boundary]

	trail := content[boundary:]
	if trail == "" {
		return s.stablePrefixRender
	}
	return glueStreamingMarkdownSurface(s.stablePrefixRender, s.renderTrailing(trail, render))
}

func (s *streamingMarkdownSurface) tryAdvanceFromEmpty(content, contextKey string, render func(string) string) {
	boundary := findSafeMarkdownBoundary(content)
	if boundary <= 0 {
		return
	}
	s.stablePrefix = content[:boundary]
	s.stablePrefixRender = render(s.stablePrefix)
	s.contextKey = contextKey
}

func (s *streamingMarkdownSurface) renderTrailing(text string, render func(string) string) string {
	if text == "" {
		return ""
	}
	return render(text)
}

func glueStreamingMarkdownSurface(prefix, trail string) string {
	prefix = strings.Trim(prefix, "\n")
	trail = strings.Trim(trail, "\n")
	switch {
	case prefix == "" && trail == "":
		return ""
	case prefix == "":
		return trail
	case trail == "":
		return prefix
	default:
		return prefix + "\n\n" + trail
	}
}

type streamingMarkdownSurfaceLRU struct {
	cap      int
	maxBytes int
	bytes    int
	tick     uint64
	entries  map[string]streamingMarkdownSurfaceEntry
}

type streamingMarkdownSurfaceEntry struct {
	state  streamingMarkdownSurface
	usedAt uint64
	size   int
}

type streamingMarkdownSurfaceCache struct {
	cache *streamingMarkdownSurfaceLRU
}

func newStreamingMarkdownSurfaceCache(cap int) *streamingMarkdownSurfaceCache {
	return &streamingMarkdownSurfaceCache{
		cache: newStreamingMarkdownSurfaceLRU(cap),
	}
}

func newStreamingMarkdownSurfaceLRU(cap int) *streamingMarkdownSurfaceLRU {
	if cap <= 0 {
		cap = lruDefaultCap
	}
	const defaultStreamingMarkdownSurfaceMaxBytes = 2 << 20
	return &streamingMarkdownSurfaceLRU{
		cap:      cap,
		maxBytes: defaultStreamingMarkdownSurfaceMaxBytes,
		entries:  make(map[string]streamingMarkdownSurfaceEntry, cap),
	}
}

func (c *streamingMarkdownSurfaceLRU) get(key string) (streamingMarkdownSurface, bool) {
	if c == nil {
		return streamingMarkdownSurface{}, false
	}
	entry, ok := c.entries[key]
	if !ok {
		return streamingMarkdownSurface{}, false
	}
	c.tick++
	entry.usedAt = c.tick
	c.entries[key] = entry
	return entry.state, true
}

func (c *streamingMarkdownSurfaceLRU) put(key string, state streamingMarkdownSurface) {
	if c == nil {
		return
	}
	size := streamingMarkdownSurfaceEntryBytes(key, state)
	if entry, ok := c.entries[key]; ok {
		c.bytes -= entry.size
		if c.bytes < 0 {
			c.bytes = 0
		}
		delete(c.entries, key)
	}
	if size > c.maxBytes {
		return
	}
	c.tick++
	entry := streamingMarkdownSurfaceEntry{
		state:  state,
		usedAt: c.tick,
		size:   size,
	}
	c.entries[key] = entry
	c.bytes += size
	for len(c.entries) > c.cap || c.bytes > c.maxBytes {
		c.evictOldest()
	}
}

func (c *streamingMarkdownSurfaceLRU) evictOldest() {
	if c == nil || len(c.entries) == 0 {
		return
	}
	oldestKey := ""
	oldestUsedAt := c.tick
	for candidate, entry := range c.entries {
		if oldestKey == "" || entry.usedAt < oldestUsedAt {
			oldestKey = candidate
			oldestUsedAt = entry.usedAt
		}
	}
	entry, ok := c.entries[oldestKey]
	if !ok {
		return
	}
	delete(c.entries, oldestKey)
	c.bytes -= entry.size
	if c.bytes < 0 {
		c.bytes = 0
	}
}

func (c *streamingMarkdownSurfaceCache) Render(streamKey, contextKey, content string, render func(string) string) string {
	streamKey = strings.TrimSpace(streamKey)
	if streamKey == "" {
		return render(content)
	}
	if c == nil || c.cache == nil {
		return render(content)
	}

	state, _ := c.cache.get(streamKey)
	rendered := state.Render(content, contextKey, render)
	c.cache.put(streamKey, state)
	return rendered
}

func streamingMarkdownSurfaceEntryBytes(key string, state streamingMarkdownSurface) int {
	return len(key) + len(state.contextKey) + len(state.stablePrefix) + len(state.stablePrefixRender)
}

func findSafeMarkdownBoundary(content string) int {
	if len(content) == 0 {
		return -1
	}
	for p := blankLineBefore(content, len(content)); p > 0; p = blankLineBefore(content, p-1) {
		if isSafeMarkdownBoundaryAt(content, p) {
			return p
		}
	}
	return -1
}

func blankLineBefore(content string, until int) int {
	if until <= 0 {
		return -1
	}
	end := until
	for end > 0 {
		nl := strings.LastIndexByte(content[:end], '\n')
		if nl < 0 {
			return -1
		}
		prev := strings.LastIndexByte(content[:nl], '\n')
		for prev >= 0 {
			gap := content[prev+1 : nl]
			if isBlankOrSpaces(gap) {
				return nl + 1
			}
			break
		}
		end = nl
	}
	return -1
}

func isBlankOrSpaces(s string) bool {
	for i := range len(s) {
		if s[i] != ' ' && s[i] != '\t' {
			return false
		}
	}
	return true
}

func isSafeMarkdownBoundaryAt(content string, p int) bool {
	prefix := content[:p]
	if countFenceLines(prefix)%2 != 0 {
		return false
	}
	if prefixHasOpenHazard(prefix) {
		return false
	}
	lastLine := lastNonBlankMarkdownLine(prefix)
	if lastLine != "" && markdownLineOpensConstruct(lastLine) {
		return false
	}
	if rest := content[p:]; rest != "" {
		first := firstNonBlankMarkdownLine(rest)
		if isSetextUnderlineCandidate(first) {
			return false
		}
	}
	return true
}

func prefixHasOpenHazard(prefix string) bool {
	inFence := false
	for line := range splitMarkdownLines(prefix) {
		if isFenceLine(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" {
			continue
		}
		if isListItemMarker(trimmed) {
			return true
		}
		if isHTMLBlockOpener(line) {
			return true
		}
		if isLinkRefDefinition(line) {
			return true
		}
	}
	return false
}

func countFenceLines(s string) int {
	count := 0
	for line := range splitMarkdownLines(s) {
		if isFenceLine(line) {
			count++
		}
	}
	return count
}

func isFenceLine(line string) bool {
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	if i >= len(line) {
		return false
	}
	c := line[i]
	if c != '`' && c != '~' {
		return false
	}
	run := 0
	for i < len(line) && line[i] == c {
		i++
		run++
	}
	return run >= 3
}

func lastNonBlankMarkdownLine(s string) string {
	last := ""
	for line := range splitMarkdownLines(s) {
		if strings.TrimSpace(line) != "" {
			last = line
		}
	}
	return last
}

func firstNonBlankMarkdownLine(s string) string {
	for line := range splitMarkdownLines(s) {
		if strings.TrimSpace(line) != "" {
			return line
		}
	}
	return ""
}

func splitMarkdownLines(s string) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		start := 0
		for i := 0; i < len(s); i++ {
			if s[i] == '\n' {
				if !yield(s[start:i]) {
					return
				}
				start = i + 1
			}
		}
		if start <= len(s)-1 {
			yield(s[start:])
		}
	}
}

func markdownLineOpensConstruct(line string) bool {
	if len(line) > 0 && line[0] == '\t' {
		return true
	}
	if strings.HasPrefix(line, "    ") {
		return true
	}

	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" {
		return false
	}
	if trimmed[0] == '>' {
		return true
	}
	if isListItemMarker(trimmed) {
		return true
	}
	if strings.ContainsRune(line, '|') {
		return true
	}
	if isSetextUnderlineCandidate(trimmed) {
		return true
	}
	return false
}

func isListItemMarker(line string) bool {
	if line == "" {
		return false
	}
	c := line[0]
	if c == '-' || c == '*' || c == '+' {
		return len(line) >= 2 && (line[1] == ' ' || line[1] == '\t')
	}
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == 0 || i > 9 || i >= len(line) {
		return false
	}
	if line[i] != '.' && line[i] != ')' {
		return false
	}
	if i+1 >= len(line) {
		return false
	}
	return line[i+1] == ' ' || line[i+1] == '\t'
}

func isSetextUnderlineCandidate(line string) bool {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i == len(line) {
		return false
	}
	c := line[i]
	if c != '=' && c != '-' {
		return false
	}
	j := i
	for j < len(line) && line[j] == c {
		j++
	}
	for j < len(line) {
		if line[j] != ' ' && line[j] != '\t' {
			return false
		}
		j++
	}
	return j-i >= 1
}

func isHTMLBlockOpener(line string) bool {
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	rest := line[i:]
	if len(rest) < 2 || rest[0] != '<' {
		return false
	}
	if strings.HasPrefix(rest, "<!--") || strings.HasPrefix(rest, "<?") || strings.HasPrefix(rest, "<![CDATA[") {
		return true
	}
	if len(rest) >= 3 && rest[1] == '!' && isASCIILetter(rest[2]) {
		return true
	}
	low := strings.ToLower(rest)
	for _, tag := range []string{"<script", "<pre", "<style", "<textarea"} {
		if strings.HasPrefix(low, tag) {
			next := byte(0)
			if len(low) > len(tag) {
				next = low[len(tag)]
			}
			if next == 0 || next == ' ' || next == '\t' || next == '>' {
				return true
			}
		}
	}
	j := 1
	if j < len(rest) && rest[j] == '/' {
		j++
	}
	if j >= len(rest) || !isASCIILetter(rest[j]) {
		return false
	}
	return true
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isLinkRefDefinition(line string) bool {
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	if i >= len(line) || line[i] != '[' {
		return false
	}
	i++
	labelStart := i
	for i < len(line) && line[i] != ']' {
		i++
	}
	if i >= len(line) || i == labelStart {
		return false
	}
	i++
	if i >= len(line) || line[i] != ':' {
		return false
	}
	i++
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return i < len(line)
}
