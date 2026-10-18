package registry

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// Registry — прикладной сервис реестра. HTTP-слой работает только через него.
type Registry struct {
	repo  Repository
	now   func() time.Time
	newID func() string
}

// New создаёт Registry поверх хранилища.
func New(repo Repository) *Registry {
	return &Registry{repo: repo, now: func() time.Time { return time.Now().UTC() }, newID: NewID}
}

// Ping проверяет доступность хранилища.
func (r *Registry) Ping(ctx context.Context) error { return r.repo.Ping(ctx) }

// Catalog возвращает элементы, которые нужно показать зрителю на витрине.
func (r *Registry) Catalog(ctx context.Context, v Viewer) ([]Item, error) {
	items, err := r.repo.List(ctx, ListFilter{Statuses: []Status{StatusActive, StatusMaintenance}})
	if err != nil {
		return nil, err
	}
	out := items[:0]
	for _, it := range items {
		if it.VisibleTo(v) {
			out = append(out, it)
		}
	}
	return out, nil
}

// CatalogItem возвращает элемент, если он виден зрителю; иначе ErrNotFound
// (не раскрываем факт существования скрытых элементов).
func (r *Registry) CatalogItem(ctx context.Context, id string, v Viewer) (Item, error) {
	it, err := r.Get(ctx, id)
	if err != nil {
		return Item{}, err
	}
	if !it.VisibleTo(v) {
		return Item{}, ErrNotFound
	}
	return it, nil
}

// List — полная выборка для управления (все статусы, с владельцами).
func (r *Registry) List(ctx context.Context, f ListFilter) ([]Item, error) {
	return r.repo.List(ctx, f)
}

// Get — элемент по id без учёта видимости.
func (r *Registry) Get(ctx context.Context, id string) (Item, error) {
	if !IsValidID(id) {
		return Item{}, ErrNotFound
	}
	return r.repo.Get(ctx, id)
}

// Create добавляет элемент. Статус по умолчанию — draft, видимость — public.
func (r *Registry) Create(ctx context.Context, it Item) (Item, error) {
	if it.Status == "" {
		it.Status = StatusDraft
	}
	if it.Visibility == "" {
		it.Visibility = VisibilityPublic
	}
	if err := normalizeAndValidate(&it); err != nil {
		return Item{}, err
	}
	now := r.now()
	it.ID = r.newID()
	it.CreatedAt, it.UpdatedAt = now, now
	if err := r.repo.Create(ctx, it); err != nil {
		return Item{}, err
	}
	return it, nil
}

// Patch — частичное обновление: nil означает «не менять».
type Patch struct {
	Slug         *string
	Name         *string
	Description  *string
	Icon         *string
	TargetType   *TargetType
	Target       *string
	Status       *Status
	Visibility   *Visibility
	AllowedRoles *[]string
	OwnerName    *string
	OwnerContact *string
	SortOrder    *int
	Pinned       *bool
}

func (p Patch) apply(it *Item) {
	set := func(dst *string, src *string) {
		if src != nil {
			*dst = *src
		}
	}
	set(&it.Slug, p.Slug)
	set(&it.Name, p.Name)
	set(&it.Description, p.Description)
	set(&it.Icon, p.Icon)
	set(&it.Target, p.Target)
	set(&it.Owner.Name, p.OwnerName)
	set(&it.Owner.Contact, p.OwnerContact)
	if p.TargetType != nil {
		it.TargetType = *p.TargetType
	}
	if p.Status != nil {
		it.Status = *p.Status
	}
	if p.Visibility != nil {
		it.Visibility = *p.Visibility
		// Переход с role_based на другую видимость без явных ролей — роли сбрасываем.
		if it.Visibility != VisibilityRoleBased && p.AllowedRoles == nil {
			it.AllowedRoles = nil
		}
	}
	if p.AllowedRoles != nil {
		it.AllowedRoles = append([]string(nil), (*p.AllowedRoles)...)
	}
	if p.SortOrder != nil {
		it.SortOrder = *p.SortOrder
	}
	if p.Pinned != nil {
		it.Pinned = *p.Pinned
	}
}

// Update применяет частичное обновление и проверяет итоговый элемент целиком.
func (r *Registry) Update(ctx context.Context, id string, p Patch) (Item, error) {
	it, err := r.Get(ctx, id)
	if err != nil {
		return Item{}, err
	}
	p.apply(&it)
	if err := normalizeAndValidate(&it); err != nil {
		return Item{}, err
	}
	it.UpdatedAt = r.now()
	if err := r.repo.Update(ctx, it); err != nil {
		return Item{}, err
	}
	return it, nil
}

// Delete удаляет элемент.
func (r *Registry) Delete(ctx context.Context, id string) error {
	if !IsValidID(id) {
		return ErrNotFound
	}
	return r.repo.Delete(ctx, id)
}

// SeedResult — итог сидирования одного элемента.
type SeedResult string

const (
	SeedCreated SeedResult = "created"
	SeedUpdated SeedResult = "updated"
	SeedSkipped SeedResult = "skipped"
)

// Seed добавляет элемент по slug. Если элемент уже есть: при overwrite=true он
// перезаписывается данными сида (id и created_at сохраняются), иначе пропускается.
// Статус и видимость по умолчанию в сиде — active/public.
func (r *Registry) Seed(ctx context.Context, it Item, overwrite bool) (SeedResult, error) {
	if it.Status == "" {
		it.Status = StatusActive
	}
	if it.Visibility == "" {
		it.Visibility = VisibilityPublic
	}
	if err := normalizeAndValidate(&it); err != nil {
		return "", fmt.Errorf("%s: %w", it.Slug, err)
	}
	existing, err := r.repo.GetBySlug(ctx, it.Slug)
	switch {
	case errors.Is(err, ErrNotFound):
		now := r.now()
		it.ID = r.newID()
		it.CreatedAt, it.UpdatedAt = now, now
		return SeedCreated, r.repo.Create(ctx, it)
	case err != nil:
		return "", err
	case !overwrite:
		return SeedSkipped, nil
	default:
		it.ID, it.CreatedAt, it.UpdatedAt = existing.ID, existing.CreatedAt, r.now()
		return SeedUpdated, r.repo.Update(ctx, it)
	}
}

var idRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// IsValidID проверяет формат id (UUID в нижнем регистре).
func IsValidID(id string) bool { return idRe.MatchString(id) }

// NewID генерирует случайный UUID v4.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // crypto/rand не должен падать
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
