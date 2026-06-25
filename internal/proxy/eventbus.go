package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// RequestEvent captures the key fields of a single proxied HTTP request.
type RequestEvent struct {
	Timestamp  time.Time `json:"ts"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	Port       int       `json:"port"`
	StatusCode int       `json:"status"`
	DurationMs int64     `json:"duration_ms"`
}

// EventBus broadcasts RequestEvents to all current subscribers.
// It is safe for concurrent use. Slow subscribers are silently dropped.
type EventBus struct {
	mu   sync.RWMutex
	subs map[chan RequestEvent]struct{}
}

// NewEventBus returns an initialised, empty EventBus.
func NewEventBus() *EventBus {
	return &EventBus{subs: make(map[chan RequestEvent]struct{})}
}

// Subscribe registers a new subscriber and returns a read channel and
// an unsubscribe function. The caller must call unsubscribe when done.
func (b *EventBus) Subscribe() (<-chan RequestEvent, func()) {
	ch := make(chan RequestEvent, 16)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
}

// Publish sends e to all subscribers. If a subscriber's buffer is full
// the event is dropped for that subscriber (non-blocking).
func (b *EventBus) Publish(e RequestEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// ServeHTTP implements http.Handler and upgrades the connection to an
// SSE stream. It writes each published RequestEvent as a "data: <JSON>\n\n"
// frame. Returns when the client disconnects (r.Context().Done()).
func (b *EventBus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch, unsub := b.Subscribe()
	defer unsub()

	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			data, err := json.Marshal(e)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
