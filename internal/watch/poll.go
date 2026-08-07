package watch

import (
	"os"
	"time"
)

const pollInterval = 300 * time.Millisecond

type poller struct {
	target string
	events chan struct{}
	done   chan struct{}
	closed chan struct{}
}

func newPoller(target string) (*poller, error) {
	p := &poller{
		target: target,
		events: make(chan struct{}, 1),
		done:   make(chan struct{}),
		closed: make(chan struct{}),
	}
	go p.loop()
	return p, nil
}

// fingerprint captures the state we compare across polls. A missing file is a
// valid state; recreation (W-4) is detected as a change back to existing.
type fingerprint struct {
	exists  bool
	size    int64
	modTime time.Time
}

func stat(target string) fingerprint {
	fi, err := os.Stat(target)
	if err != nil {
		return fingerprint{exists: false}
	}
	return fingerprint{exists: true, size: fi.Size(), modTime: fi.ModTime()}
}

func (p *poller) loop() {
	defer close(p.closed)
	defer close(p.events)
	prev := stat(p.target)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
			cur := stat(p.target)
			if cur != prev {
				prev = cur
				if cur.exists {
					p.emit()
				}
			}
		}
	}
}

func (p *poller) emit() {
	select {
	case p.events <- struct{}{}:
	default:
	}
}

func (p *poller) Events() <-chan struct{} { return p.events }

func (p *poller) Close() error {
	select {
	case <-p.done:
	default:
		close(p.done)
	}
	<-p.closed
	return nil
}
