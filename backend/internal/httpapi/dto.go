package httpapi

import (
	"time"

	"mgu-showcase/backend/internal/registry"
)

// catalogItem — то, что получает фронтенд витрины. Без владельца и правил доступа.
type catalogItem struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	TargetType  string `json:"target_type"`
	Target      string `json:"target"`
	Status      string `json:"status"` // active | maintenance
	Pinned      bool   `json:"pinned"`
}

type ownerDTO struct {
	Name    string `json:"name"`
	Contact string `json:"contact"`
}

// fullItem — полное представление для управления реестром (scope=all).
type fullItem struct {
	catalogItem
	Visibility   string    `json:"visibility"`
	AllowedRoles []string  `json:"allowed_roles"`
	Owner        ownerDTO  `json:"owner"`
	SortOrder    int       `json:"sort_order"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func toCatalog(it registry.Item) catalogItem {
	return catalogItem{
		ID: it.ID, Slug: it.Slug, Name: it.Name, Description: it.Description, Icon: it.Icon,
		TargetType: string(it.TargetType), Target: it.Target, Status: string(it.Status), Pinned: it.Pinned,
	}
}

func toFull(it registry.Item) fullItem {
	roles := it.AllowedRoles
	if roles == nil {
		roles = []string{}
	}
	return fullItem{
		catalogItem:  toCatalog(it),
		Visibility:   string(it.Visibility),
		AllowedRoles: roles,
		Owner:        ownerDTO{Name: it.Owner.Name, Contact: it.Owner.Contact},
		SortOrder:    it.SortOrder,
		CreatedAt:    it.CreatedAt,
		UpdatedAt:    it.UpdatedAt,
	}
}

type listResponse[T any] struct {
	Items []T `json:"items"`
}

// createRequest — тело POST /api/v1/services.
type createRequest struct {
	Slug         string   `json:"slug"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Icon         string   `json:"icon"`
	TargetType   string   `json:"target_type"`
	Target       string   `json:"target"`
	Status       string   `json:"status"`     // по умолчанию draft
	Visibility   string   `json:"visibility"` // по умолчанию public
	AllowedRoles []string `json:"allowed_roles"`
	Owner        ownerDTO `json:"owner"`
	SortOrder    int      `json:"sort_order"`
	Pinned       bool     `json:"pinned"`
}

func (r createRequest) item() registry.Item {
	return registry.Item{
		Slug: r.Slug, Name: r.Name, Description: r.Description, Icon: r.Icon,
		TargetType: registry.TargetType(r.TargetType), Target: r.Target,
		Status: registry.Status(r.Status), Visibility: registry.Visibility(r.Visibility),
		AllowedRoles: r.AllowedRoles,
		Owner:        registry.Owner{Name: r.Owner.Name, Contact: r.Owner.Contact},
		SortOrder:    r.SortOrder, Pinned: r.Pinned,
	}
}

// patchRequest — тело PATCH: передаются только изменяемые поля.
type patchRequest struct {
	Slug         *string   `json:"slug"`
	Name         *string   `json:"name"`
	Description  *string   `json:"description"`
	Icon         *string   `json:"icon"`
	TargetType   *string   `json:"target_type"`
	Target       *string   `json:"target"`
	Status       *string   `json:"status"`
	Visibility   *string   `json:"visibility"`
	AllowedRoles *[]string `json:"allowed_roles"`
	Owner        *struct {
		Name    *string `json:"name"`
		Contact *string `json:"contact"`
	} `json:"owner"`
	SortOrder *int  `json:"sort_order"`
	Pinned    *bool `json:"pinned"`
}

func (r patchRequest) patch() registry.Patch {
	p := registry.Patch{
		Slug: r.Slug, Name: r.Name, Description: r.Description, Icon: r.Icon, Target: r.Target,
		AllowedRoles: r.AllowedRoles, SortOrder: r.SortOrder, Pinned: r.Pinned,
	}
	if r.TargetType != nil {
		v := registry.TargetType(*r.TargetType)
		p.TargetType = &v
	}
	if r.Status != nil {
		v := registry.Status(*r.Status)
		p.Status = &v
	}
	if r.Visibility != nil {
		v := registry.Visibility(*r.Visibility)
		p.Visibility = &v
	}
	if r.Owner != nil {
		p.OwnerName, p.OwnerContact = r.Owner.Name, r.Owner.Contact
	}
	return p
}
