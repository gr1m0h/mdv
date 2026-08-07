// Package watch reports changes to a single target file. It watches the parent
// directory (not the file itself) so it survives editors that save via
// write-temp-then-rename, and falls back to stat polling when fsnotify is
// unavailable or MDV_WATCH=poll is set. See spec §7.2.
package watch

import (
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

const debounce = 120 * time.Millisecond

// Watcher reports changes to a target file over Events until Close is called.
type Watcher interface {
	// Events yields once per debounced change to the target file. The channel
	// is closed when the watcher stops.
	Events() <-chan struct{}
	// Close releases all resources and stops the watcher.
	Close() error
}

// New creates a Watcher for target. mode "poll" forces polling; any other value
// tries fsnotify first and falls back to polling on failure.
func New(target, mode string) (Watcher, error) {
	target = filepath.Clean(target)
	if mode == "poll" {
		return newPoller(target)
	}
	w, err := newFSWatcher(target)
	if err != nil {
		return newPoller(target)
	}
	return w, nil
}

type fsWatcher struct {
	target string
	w      *fsnotify.Watcher
	events chan struct{}
	done   chan struct{}
	closed chan struct{}
}

func newFSWatcher(target string) (*fsWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := w.Add(filepath.Dir(target)); err != nil {
		w.Close()
		return nil, err
	}
	fw := &fsWatcher{
		target: target,
		w:      w,
		events: make(chan struct{}, 1),
		done:   make(chan struct{}),
		closed: make(chan struct{}),
	}
	go fw.loop()
	return fw, nil
}

func (f *fsWatcher) loop() {
	defer close(f.closed)
	defer close(f.events)
	var timer *time.Timer
	var timerC <-chan time.Time
	for {
		select {
		case <-f.done:
			if timer != nil {
				timer.Stop()
			}
			return
		case ev, ok := <-f.w.Events:
			if !ok {
				return
			}
			if filepath.Clean(ev.Name) != f.target {
				continue
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}
			if timer == nil {
				timer = time.NewTimer(debounce)
			} else {
				timer.Reset(debounce)
			}
			timerC = timer.C
		case <-timerC:
			timerC = nil
			f.emit()
		case _, ok := <-f.w.Errors:
			if !ok {
				return
			}
		}
	}
}

func (f *fsWatcher) emit() {
	select {
	case f.events <- struct{}{}:
	default:
	}
}

func (f *fsWatcher) Events() <-chan struct{} { return f.events }

func (f *fsWatcher) Close() error {
	select {
	case <-f.done:
	default:
		close(f.done)
	}
	err := f.w.Close()
	<-f.closed
	return err
}
