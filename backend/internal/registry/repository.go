package registry

import (
	"context"
	"errors"
)

var (
	// ErrNotFound — элемент не найден (или не виден текущему зрителю).
	ErrNotFound = errors.New("registry: item not found")
	// ErrConflict — нарушена уникальность (например, slug уже занят).
	ErrConflict = errors.New("registry: item conflicts with existing one")
)

// ListFilter — фильтр выборки. Пустые поля не ограничивают выборку.
type ListFilter struct {
	Statuses []Status
}

// Repository — хранилище реестра. Реализации: pgstore (Postgres), memstore (в памяти).
//
// Списки возвращаются отсортированными: pinned desc, sort_order asc, name asc, id asc.
type Repository interface {
	List(ctx context.Context, f ListFilter) ([]Item, error)
	Get(ctx context.Context, id string) (Item, error)
	GetBySlug(ctx context.Context, slug string) (Item, error)
	Create(ctx context.Context, it Item) error
	Update(ctx context.Context, it Item) error
	Delete(ctx context.Context, id string) error
	Ping(ctx context.Context) error
}
