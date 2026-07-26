package kuma

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
// listState holds one such list and lets callers block until the next push
// arrives, which is how a mutation is turned into a consistent read.
type listState[T any] struct {
	mu      sync.RWMutex
	items   map[int]T
	loaded  bool
	updated chan struct{}
}

func newListState[T any]() *listState[T] {
	return &listState[T]{
		items:   make(map[int]T),
		updated: make(chan struct{}),
	}
}

// replace swaps the whole list, as the *List events do.
func (s *listState[T]) replace(items map[int]T) {
	s.mu.Lock()
	s.items = items
	s.loaded = true
	s.signalLocked()
	s.mu.Unlock()
}

// patch merges entries into the list without dropping the others. The server
// sends partial updates for monitors (updateMonitorIntoList) rather than
// resending everything after an edit.
func (s *listState[T]) patch(items map[int]T) {
	s.mu.Lock()
	for id, item := range items {
		s.items[id] = item
	}
	s.loaded = true
	s.signalLocked()
	s.mu.Unlock()
}

func (s *listState[T]) remove(id int) {
	s.mu.Lock()
	delete(s.items, id)
	s.signalLocked()
	s.mu.Unlock()
}

// invalidate marks the list as unloaded. Called on reconnect, since the cached
// contents may predate changes made by someone else while we were away.
func (s *listState[T]) invalidate() {
	s.mu.Lock()
	s.items = make(map[int]T)
	s.loaded = false
	s.mu.Unlock()
}

// signalLocked wakes every waiter and arms a fresh channel for the next round.
// Callers must hold the write lock.
func (s *listState[T]) signalLocked() {
	close(s.updated)
	s.updated = make(chan struct{})
}

// subscribe returns the channel closed by the next push. Take it *before*
// emitting the event that triggers the push, otherwise the update can land in
// the gap between the acknowledgement and the subscription.
func (s *listState[T]) subscribe() <-chan struct{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.updated
}

func (s *listState[T]) get(id int) (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	return item, ok
}

func (s *listState[T]) all() map[int]T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[int]T, len(s.items))
	for id, item := range s.items {
		out[id] = item
	}
	return out
}

func (s *listState[T]) isLoaded() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loaded
}

// waitUpdate blocks until the given channel is closed, the context ends, or the
// deadline passes. A missed push is not fatal — the caller falls back to
// whatever the cache already holds — so the timeout returns nil.
func waitUpdate(ctx context.Context, ch <-chan struct{}) {
	select {
	case <-ch:
	case <-ctx.Done():
	}
}

// cache groups every pushed list plus the server info payload.
type cache struct {
	monitors       *listState[Monitor]
	maintenances   *listState[Maintenance]
	notifications  *listState[Notification]
	proxies        *listState[Proxy]
	dockerHosts    *listState[DockerHost]
	remoteBrowsers *listState[RemoteBrowser]
	apiKeys        *listState[APIKey]
	statusPages    *listState[StatusPage]

	infoMu sync.RWMutex
	info   ServerInfo
}

func newCache() *cache {
	return &cache{
		monitors:       newListState[Monitor](),
		maintenances:   newListState[Maintenance](),
		notifications:  newListState[Notification](),
		proxies:        newListState[Proxy](),
		dockerHosts:    newListState[DockerHost](),
		remoteBrowsers: newListState[RemoteBrowser](),
		apiKeys:        newListState[APIKey](),
		statusPages:    newListState[StatusPage](),
	}
}

func (c *cache) invalidate() {
	c.monitors.invalidate()
	c.maintenances.invalidate()
	c.notifications.invalidate()
	c.proxies.invalidate()
	c.dockerHosts.invalidate()
	c.remoteBrowsers.invalidate()
	c.apiKeys.invalidate()
	c.statusPages.invalidate()
}

func (c *cache) setInfo(info ServerInfo) {
	c.infoMu.Lock()
	c.info = info
	c.infoMu.Unlock()
}

func (c *cache) getInfo() ServerInfo {
	c.infoMu.RLock()
	defer c.infoMu.RUnlock()
	return c.info
}
