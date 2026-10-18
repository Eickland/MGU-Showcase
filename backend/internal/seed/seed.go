// Package seed загружает начальные записи реестра из JSON-файла.
package seed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"mgu-showcase/backend/internal/registry"
)

// File — формат файла сида (см. seed/services.json).
type File struct {
	Items []Entry `json:"items"`
}

type Entry struct {
	Slug         string   `json:"slug"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Icon         string   `json:"icon"`
	TargetType   string   `json:"target_type"`
	Target       string   `json:"target"`
	Status       string   `json:"status"`
	Visibility   string   `json:"visibility"`
	AllowedRoles []string `json:"allowed_roles"`
	Owner        struct {
		Name    string `json:"name"`
		Contact string `json:"contact"`
	} `json:"owner"`
	SortOrder int  `json:"sort_order"`
	Pinned    bool `json:"pinned"`
}

func (e Entry) item() registry.Item {
	return registry.Item{
		Slug: e.Slug, Name: e.Name, Description: e.Description, Icon: e.Icon,
		TargetType: registry.TargetType(e.TargetType), Target: e.Target,
		Status: registry.Status(e.Status), Visibility: registry.Visibility(e.Visibility),
		AllowedRoles: e.AllowedRoles,
		Owner:        registry.Owner{Name: e.Owner.Name, Contact: e.Owner.Contact},
		SortOrder:    e.SortOrder, Pinned: e.Pinned,
	}
}

// Load читает и разбирает файл сида.
func Load(path string) (File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var f File
	if err := dec.Decode(&f); err != nil {
		return File{}, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

// Apply добавляет записи в реестр (идемпотентно, по slug).
// overwrite=true перезаписывает уже существующие записи данными из файла.
func Apply(ctx context.Context, reg *registry.Registry, f File, overwrite bool, log *slog.Logger) error {
	counts := map[registry.SeedResult]int{}
	for _, e := range f.Items {
		res, err := reg.Seed(ctx, e.item(), overwrite)
		if err != nil {
			return fmt.Errorf("seed %q: %w", e.Slug, err)
		}
		counts[res]++
	}
	log.Info("seed applied",
		"created", counts[registry.SeedCreated],
		"updated", counts[registry.SeedUpdated],
		"skipped", counts[registry.SeedSkipped])
	return nil
}
