package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEventBus_PublishDelivers(t *testing.T) {
	bus := NewEventBus()
	ch, unsub := bus.Subscribe()
	defer unsub()

	want := RequestEvent{Method: "GET", Path: "/test", Port: 3000, StatusCode: 200}
	bus.Publish(want)

	select {
	case got := <-ch:
		if got.Method != want.Method || got.Path != want.Path || got.Port != want.Port {
			t.Errorf("got %+v, want %+v", got, want)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout: event not received")
	}
}

func TestEventBus_UnsubscribeStopsDelivery(t *testing.T) {
	bus := NewEventBus()
	ch, unsub := bus.Subscribe()
	unsub()

	bus.Publish(RequestEvent{Method: "GET", Port: 3000})

	select {
	case <-ch:
		t.Fatal("received event after unsubscribe")
	case <-time.After(50 * time.Millisecond):
		// correct: no event
	}
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	bus := NewEventBus()
	ch1, unsub1 := bus.Subscribe()
	ch2, unsub2 := bus.Subscribe()
	defer unsub1()
	defer unsub2()

	bus.Publish(RequestEvent{Method: "POST", Port: 8080, StatusCode: 201})

	for _, ch := range []<-chan RequestEvent{ch1, ch2} {
		select {
		case got := <-ch:
			if got.Method != "POST" {
				t.Errorf("want POST, got %s", got.Method)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timeout: event not received by subscriber")
		}
	}
}

func TestEventBus_SlowClientDoesNotBlock(t *testing.T) {
	bus := NewEventBus()
	_, unsub := bus.Subscribe() // subscribe but never read
	defer unsub()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 20; i++ {
			bus.Publish(RequestEvent{Method: "GET", Port: 3000})
		}
		close(done)
	}()

	select {
	case <-done:
		// correct: publish did not block
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Publish blocked on slow subscriber")
	}
}

func TestEventBus_ServeHTTP_StreamsEvent(t *testing.T) {
	bus := NewEventBus()

	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		bus.ServeHTTP(rr, req)
		close(done)
	}()

	// Give the handler time to subscribe
	time.Sleep(10 * time.Millisecond)

	bus.Publish(RequestEvent{Method: "GET", Path: "/health", Port: 3000, StatusCode: 200})
	time.Sleep(20 * time.Millisecond)

	cancel()
	<-done

	body := rr.Body.String()
	if ct := rr.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("want Content-Type text/event-stream, got %q", ct)
	}

	var foundEvent bool
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data: ") {
			var e RequestEvent
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &e); err != nil {
				t.Errorf("could not parse event JSON: %v", err)
			}
			if e.Method == "GET" && e.Path == "/health" {
				foundEvent = true
			}
		}
	}
	if !foundEvent {
		t.Errorf("expected GET /health event in SSE body, got: %q", body)
	}
}
