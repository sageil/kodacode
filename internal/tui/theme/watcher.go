package theme

import (
	"log"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// Watcher monitors a theme file and calls onChange with the new Theme on valid writes.
type Watcher struct {
	fw        *fsnotify.Watcher
	onChange  func(*Theme)
	done      chan struct{}
	closeOnce sync.Once
}

// NewWatcher starts watching path. onChange is called on every valid file write.
func NewWatcher(path string, onChange func(*Theme)) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := fw.Add(path); err != nil {
		_ = fw.Close()
		return nil, err
	}
	w := &Watcher{fw: fw, onChange: onChange, done: make(chan struct{})}
	go w.loop()
	return w, nil
}

func (w *Watcher) loop() {
	for {
		select {
		case <-w.done:
			return
		case event, ok := <-w.fw.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				th, err := NewLoader(LoaderConfig{Path: event.Name}).Load()
				if err != nil {
					log.Printf("theme watcher: invalid theme file: %v", err)
					continue
				}
				w.onChange(th)
			}
		case err, ok := <-w.fw.Errors:
			if !ok {
				return
			}
			log.Printf("theme watcher: %v", err)
		}
	}
}

// Close stops the watcher. Safe to call multiple times.
func (w *Watcher) Close() {
	w.closeOnce.Do(func() {
		close(w.done)
		_ = w.fw.Close()
	})
}
