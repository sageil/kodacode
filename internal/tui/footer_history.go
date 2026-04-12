package tui

import "strings"

const maxHistory = 20

func (f *Footer) PushHistory(text string) {
	if text == "" {
		return
	}
	// Avoid duplicate consecutive entries.
	if len(f.history) > 0 && f.history[len(f.history)-1] == text {
		f.historyIdx = len(f.history)
		return
	}
	f.history = append(f.history, text)
	// Cap at maxHistory entries.
	if len(f.history) > maxHistory {
		f.history = f.history[len(f.history)-maxHistory:]
	}
	f.historyIdx = len(f.history)
}

// History returns the current history entries for persistence.
func (f *Footer) History() []string { return f.history }

// LoadHistory replaces the in-memory history with persisted entries.
func (f *Footer) LoadHistory(entries []string) {
	if len(entries) > maxHistory {
		entries = entries[len(entries)-maxHistory:]
	}
	f.history = entries
	f.historyIdx = len(f.history)
}

// searchHistory finds the most recent history entry containing the query.
func searchHistory(history []string, query string) string {
	q := strings.ToLower(query)
	for i := len(history) - 1; i >= 0; i-- {
		if strings.Contains(strings.ToLower(history[i]), q) {
			return history[i]
		}
	}
	return ""
}
