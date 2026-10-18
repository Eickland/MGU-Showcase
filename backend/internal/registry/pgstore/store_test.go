package pgstore_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"

	"mgu-showcase/backend/internal/db"
	"mgu-showcase/backend/internal/registry"
	"mgu-showcase/backend/internal/registry/pgstore"
)

// Интеграционный тест. Запуск: TEST_DATABASE_URL=postgres://... go test ./...
// ВНИМАНИЕ: очищает таблицу registry_items в указанной БД.
func TestPostgresStore(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL не задан")
	}
	ctx := context.Background()
	conn, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := db.Migrate(ctx, conn, log); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx, conn, log); err != nil { // повторно — без ошибок
		t.Fatal(err)
	}
	if _, err := conn.Exec(`DELETE FROM registry_items`); err != nil {
		t.Fatal(err)
	}

	reg := registry.New(pgstore.New(conn))
	base := registry.Item{
		Slug: "a", Name: "A", TargetType: registry.TargetExternalURL, Target: "https://a.msu.ru",
		Status: registry.StatusActive, Visibility: registry.VisibilityPublic, Owner: registry.Owner{Name: "team"},
	}
	a, err := reg.Create(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	b := base
	b.Slug, b.Name, b.Pinned = "b", "B", true
	b.Visibility, b.AllowedRoles = registry.VisibilityRoleBased, []string{"staff"}
	if _, err := reg.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Create(ctx, base); !errors.Is(err, registry.ErrConflict) {
		t.Fatalf("want conflict, got %v", err)
	}

	all, err := reg.List(ctx, registry.ListFilter{})
	if err != nil || len(all) != 2 || all[0].Slug != "b" || all[0].AllowedRoles[0] != "staff" {
		t.Fatalf("list (pinned first): %+v %v", all, err)
	}
	cat, _ := reg.Catalog(ctx, registry.Anonymous)
	if len(cat) != 1 || cat[0].ID != a.ID {
		t.Fatalf("catalog: %+v", cat)
	}

	st := registry.StatusDraft
	if _, err := reg.Update(ctx, a.ID, registry.Patch{Status: &st}); err != nil {
		t.Fatal(err)
	}
	if drafts, _ := reg.List(ctx, registry.ListFilter{Statuses: []registry.Status{registry.StatusDraft}}); len(drafts) != 1 {
		t.Fatalf("status filter: %+v", drafts)
	}
	got, err := reg.Get(ctx, a.ID)
	if err != nil || got.Status != registry.StatusDraft || len(got.AllowedRoles) != 0 || !got.UpdatedAt.After(got.CreatedAt) {
		t.Fatalf("get: %+v %v", got, err)
	}
	if err := reg.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Get(ctx, a.ID); !errors.Is(err, registry.ErrNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
}
