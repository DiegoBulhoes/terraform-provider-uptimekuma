package wire

import (
	"context"
	"sync"
)

// Uptime Kuma has no getter for several entities. Notifications, proxies,
// docker hosts, remote browsers and API keys are only ever delivered by a
// server-initiated push: the server emits the full list after login and again
// after every mutation. Monitors and maintenances have getters, but those
// getters answer `{ok: true}` and push the payload through the same channel.
//
// So the only way to read state is to keep the pushed lists and consult them.
// ListState holds one such list and lets callers block until the next push
// arrives, which is how a mutation is turned into a consistent read.
type ListState[T any] struct {
	mu      sync.RWMutex
	items   map[int]T
	loaded  bool
	updated chan struct{}
}

func NewListState[T any]() *ListState[T] {
	return &ListState[T]{
		items:   make(map[int]T),
		updated: make(chan struct{}),
	}
}

// replace swaps the whole list, as the *List events do.
func (s *ListState[T]) Replace(items map[int]T) {
	s.mu.Lock()
	s.items = items
	s.loaded = true
	s.signalLocked()
	s.mu.Unlock()
}

// patch merges entries into the list without dropping the others. The server
// sends partial updates for monitors (updateMonitorIntoList) rather than
// resending everything after an edit.
func (s *ListState[T]) Patch(items map[int]T) {
	s.mu.Lock()
	for id, item := range items {
		s.items[id] = item
	}
	s.loaded = true
	s.signalLocked()
	s.mu.Unlock()
}

func (s *ListState[T]) Remove(id int) {
	s.mu.Lock()
	delete(s.items, id)
	s.signalLocked()
	s.mu.Unlock()
}

// invalidate marks the list as unloaded. Called on reconnect, since the cached
// contents may predate changes made by someone else while we were away.
func (s *ListState[T]) Invalidate() {
	s.mu.Lock()
	s.items = make(map[int]T)
	s.loaded = false
	s.mu.Unlock()
}

// signalLocked wakes every waiter and arms a fresh channel for the next round.
// Callers must hold the write lock.
func (s *ListState[T]) signalLocked() {
	close(s.updated)
	s.updated = make(chan struct{})
}

// subscribe returns the channel closed by the next push. Take it *before*
// emitting the event that triggers the push, otherwise the update can land in
// the gap between the acknowledgement and the subscription.
func (s *ListState[T]) Subscribe() <-chan struct{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.updated
}

func (s *ListState[T]) Has(id int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.items[id]
	return ok
}

func (s *ListState[T]) Get(id int) (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	return item, ok
}

func (s *ListState[T]) All() map[int]T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[int]T, len(s.items))
	for id, item := range s.items {
		out[id] = item
	}
	return out
}

func (s *ListState[T]) IsLoaded() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loaded
}

// WaitUpdate blocks until the given channel is closed, the context ends, or the
// deadline passes. A missed push is not fatal — the caller falls back to
// whatever the Cache already holds — so the timeout returns nil.
func WaitUpdate(ctx context.Context, ch <-chan struct{}) {
	select {
	case <-ch:
	case <-ctx.Done():
	}
}

// Cache groups every pushed list plus the server info payload.
// PushedList is the read side of a server-pushed list, shared by every entity
// whose state only arrives through push events.
type PushedList interface {
	IsLoaded() bool
	Subscribe() <-chan struct{}
	Has(id int) bool
}

type Cache struct {
	Monitors       *ListState[Monitor]
	Maintenances   *ListState[Maintenance]
	Notifications  *ListState[Notification]
	Proxies        *ListState[Proxy]
	DockerHosts    *ListState[DockerHost]
	RemoteBrowsers *ListState[RemoteBrowser]
	APIKeys        *ListState[APIKey]
	StatusPages    *ListState[StatusPage]

	infoMu sync.RWMutex
	info   ServerInfo
}

func NewCache() *Cache {
	return &Cache{
		Monitors:       NewListState[Monitor](),
		Maintenances:   NewListState[Maintenance](),
		Notifications:  NewListState[Notification](),
		Proxies:        NewListState[Proxy](),
		DockerHosts:    NewListState[DockerHost](),
		RemoteBrowsers: NewListState[RemoteBrowser](),
		APIKeys:        NewListState[APIKey](),
		StatusPages:    NewListState[StatusPage](),
	}
}

func (c *Cache) Invalidate() {
	c.Monitors.Invalidate()
	c.Maintenances.Invalidate()
	c.Notifications.Invalidate()
	c.Proxies.Invalidate()
	c.DockerHosts.Invalidate()
	c.RemoteBrowsers.Invalidate()
	c.APIKeys.Invalidate()
	c.StatusPages.Invalidate()
}

func (c *Cache) SetInfo(info ServerInfo) {
	c.infoMu.Lock()
	c.info = info
	c.infoMu.Unlock()
}

func (c *Cache) GetInfo() ServerInfo {
	c.infoMu.RLock()
	defer c.infoMu.RUnlock()
	return c.info
}
