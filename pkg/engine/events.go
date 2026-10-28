package engine

import (
	"fmt"
	"sync"
	"time"
)

// SignalEvent represents a dispatched event/signal in the OSMU engine.
type SignalEvent struct {
	Signal    string        `json:"signal"`
	SourceID  string        `json:"source_id"`
	TargetID  string        `json:"target_id,omitempty"`
	Args      []interface{} `json:"args,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
	Frame     uint64        `json:"frame"`
}

func (e SignalEvent) Summary() string {
	src := e.SourceID
	if src == "" {
		src = "global"
	}
	if len(e.Args) == 0 {
		return fmt.Sprintf("[%s] %s()", src, e.Signal)
	}
	return fmt.Sprintf("[%s] %s(%v)", src, e.Signal, e.Args)
}

// SignalHandler is a callback invoked when a signal is emitted.
type SignalHandler func(ev SignalEvent)

type subscription struct {
	id       int
	signal   string
	targetID string
	handler  SignalHandler
}

// EventBus manages typed signals, cross-entity subscriptions, and live trace history.
type EventBus struct {
	mu          sync.RWMutex
	nextSubID   int
	subs        []subscription
	history     []SignalEvent
	maxHistory  int
	lastFiredAt map[string]time.Time
	frame       uint64
}

func NewEventBus() *EventBus {
	return &EventBus{
		maxHistory:  120,
		lastFiredAt: make(map[string]time.Time),
	}
}

// SetFrame updates the current frame counter for event stamping.
func (b *EventBus) SetFrame(frame uint64) {
	b.mu.Lock()
	b.frame = frame
	b.mu.Unlock()
}

// On subscribes a handler to a signal name. If targetID is empty, it receives from all sources.
func (b *EventBus) On(signal string, targetID string, handler SignalHandler) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextSubID++
	id := b.nextSubID
	b.subs = append(b.subs, subscription{
		id:       id,
		signal:   signal,
		targetID: targetID,
		handler:  handler,
	})
	return id
}

// ClearSubscriptions removes all dynamic runtime subscriptions (e.g., on scene reload).
func (b *EventBus) ClearSubscriptions() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs = nil
}

// Emit dispatches a signal to all matching subscribers and records it in the trace log.
func (b *EventBus) Emit(signal string, sourceID string, args ...interface{}) {
	now := time.Now()
	b.mu.Lock()
	ev := SignalEvent{
		Signal:    signal,
		SourceID:  sourceID,
		Args:      args,
		Timestamp: now,
		Frame:     b.frame,
	}
	b.history = append(b.history, ev)
	if len(b.history) > b.maxHistory {
		b.history = b.history[len(b.history)-b.maxHistory:]
	}
	b.lastFiredAt[signal] = now
	if sourceID != "" {
		b.lastFiredAt[sourceID+":"+signal] = now
	}
	matching := make([]SignalHandler, 0, len(b.subs))
	for _, s := range b.subs {
		if s.signal == signal || s.signal == "*" {
			if s.targetID == "" || s.targetID == sourceID {
				matching = append(matching, s.handler)
			}
		}
	}
	b.mu.Unlock()

	for _, h := range matching {
		h(ev)
	}
}

// RecentEvents returns up to n most recent SignalEvents.
func (b *EventBus) RecentEvents(n int) []SignalEvent {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if n <= 0 || n > len(b.history) {
		n = len(b.history)
	}
	out := make([]SignalEvent, n)
	copy(out, b.history[len(b.history)-n:])
	return out
}

// WasRecentlyFired returns true if the signal fired within the given duration.
func (b *EventBus) WasRecentlyFired(signal string, within time.Duration) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	t, ok := b.lastFiredAt[signal]
	if !ok {
		return false
	}
	return time.Since(t) <= within
}
