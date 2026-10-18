// Package memstore — хранилище реестра в памяти. Для тестов и локального
// запуска без Postgres (STORAGE=memory). Данные теряются при перезапуске.
package memstore

import (
	"context"
	"slices"
	"sort"
	"sync"

	"mgu-showcase/backend/internal/registry"
)

type Store struct {
	mu    sync.RWMutex
	items map[string]registry.Item
}

func New() *Store { return &Store{items: map[string]registry.Item{}} }

var _ registry.Repository = (*Store)(nil)

func clone(it registry.Item) registry.Item {
	it.AllowedRoles = slices.Clone(it.AllowedRoles)
	return it
}

func (s *Store) List(_ context.Context, f registry.ListFilter) ([]registry.Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]registry.Item, 0, len(s.items))
	for _, it := range s.items {
		if len(f.Statuses) > 0 && !slices.Contains(f.Statuses, it.Status) {
			continue
		}
		out = append(out, clone(it))
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Pinned != b.Pinned {
			return a.Pinned
		}
		if a.SortOrder != b.SortOrder {
			return a.SortOrder < b.SortOrder
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})
	return out, nil
}

func (s *Store) Get(_ context.Context, id string) (registry.Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	it, ok := s.items[id]
	if !ok {
		return registry.Item{}, registry.ErrNotFound
	}
	return clone(it), nil
}

func (s *Store) GetBySlug(_ context.Context, slug string) (registry.Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, it := range s.items {
		if it.Slug == slug {
			return clone(it), nil
		}
	}
	return registry.Item{}, registry.ErrNotFound
}

func (s *Store) slugTaken(slug, exceptID string) bool {
	for id, it := range s.items {
		if it.Slug == slug && id != exceptID {
			return true
		}
	}
	return false
}

func (s *Store) Create(_ context.Context, it registry.Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[it.ID]; ok || s.slugTaken(it.Slug, "") {
		return registry.ErrConflict
	}
	s.items[it.ID] = clone(it)
	return nil
}

func (s *Store) Update(_ context.Context, it registry.Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[it.ID]; !ok {
		return registry.ErrNotFound
	}
	if s.slugTaken(it.Slug, it.ID) {
		return registry.ErrConflict
	}
	s.items[it.ID] = clone(it)
	return nil
}

func (s *Store) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return registry.ErrNotFound
	}
	delete(s.items, id)
	return nil
}

func (s *Store) Ping(context.Context) error { return nil }
