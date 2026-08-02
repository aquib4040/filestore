package telegram

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrListenerTimeout = errors.New("timeout waiting for user response")

type StateRegistry struct {
	mu           sync.RWMutex
	listeners    map[int64]chan string
	lastInputIDs map[int64]int
}

func NewStateRegistry() *StateRegistry {
	return &StateRegistry{
		listeners:    make(map[int64]chan string),
		lastInputIDs: make(map[int64]int),
	}
}

func (sr *StateRegistry) GetLastInputMsgID(userID int64) int {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.lastInputIDs[userID]
}

func (sr *StateRegistry) Register(userID int64) chan string {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	// If there's an existing listener, close it
	if ch, exists := sr.listeners[userID]; exists {
		close(ch)
	}

	ch := make(chan string, 1)
	sr.listeners[userID] = ch
	return ch
}

func (sr *StateRegistry) Unregister(userID int64) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if ch, exists := sr.listeners[userID]; exists {
		close(ch)
		delete(sr.listeners, userID)
	}
	delete(sr.lastInputIDs, userID)
}

func (sr *StateRegistry) Listen(ctx context.Context, userID int64, duration time.Duration) (string, error) {
	ch := sr.Register(userID)
	defer sr.Unregister(userID)

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case val, ok := <-ch:
		if !ok {
			return "", errors.New("listener closed")
		}
		return val, nil
	case <-timer.C:
		return "", ErrListenerTimeout
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (sr *StateRegistry) Push(userID int64, val string, msgID int) bool {
	sr.mu.Lock()
	sr.lastInputIDs[userID] = msgID
	ch, exists := sr.listeners[userID]
	sr.mu.Unlock()

	if !exists {
		return false
	}

	select {
	case ch <- val:
		return true
	default:
		// Channel buffer is full, do not block
		return false
	}
}
