package main

import (
	"testing"

	"github.com/tunnel-ops/tunnel/internal/proxy"
)

func TestWatchModel_FilterApplied(t *testing.T) {
	m := watchModel{portFilter: 3000}

	next, _ := m.Update(requestEventMsg{event: proxy.RequestEvent{Method: "GET", Port: 3000}})
	m1 := next.(watchModel)
	if len(m1.requests) != 1 {
		t.Fatalf("expected 1 request after matching event, got %d", len(m1.requests))
	}

	next, _ = m1.Update(requestEventMsg{event: proxy.RequestEvent{Method: "GET", Port: 8080}})
	m2 := next.(watchModel)
	if len(m2.requests) != 1 {
		t.Errorf("expected port 8080 event to be filtered out, got %d requests", len(m2.requests))
	}
}

func TestWatchModel_NoFilter_AcceptsAllPorts(t *testing.T) {
	m := watchModel{portFilter: 0}

	next, _ := m.Update(requestEventMsg{event: proxy.RequestEvent{Method: "GET", Port: 3000}})
	next, _ = next.(watchModel).Update(requestEventMsg{event: proxy.RequestEvent{Method: "POST", Port: 8080}})
	result := next.(watchModel)

	if len(result.requests) != 2 {
		t.Errorf("expected 2 requests, got %d", len(result.requests))
	}
}

func TestWatchModel_RequestsCapped(t *testing.T) {
	var current watchModel
	for i := 0; i < maxRequests+10; i++ {
		next, _ := current.Update(requestEventMsg{
			event: proxy.RequestEvent{Method: "GET", Port: 3000},
		})
		current = next.(watchModel)
	}
	if len(current.requests) > maxRequests {
		t.Errorf("requests not capped: got %d, want max %d", len(current.requests), maxRequests)
	}
}

func TestWatchModel_NewestFirst(t *testing.T) {
	m := watchModel{}

	next, _ := m.Update(requestEventMsg{event: proxy.RequestEvent{Path: "/first", Port: 3000}})
	next, _ = next.(watchModel).Update(requestEventMsg{event: proxy.RequestEvent{Path: "/second", Port: 3000}})
	result := next.(watchModel)

	if result.requests[0].Path != "/second" {
		t.Errorf("expected newest request first, got path %q", result.requests[0].Path)
	}
}
