package registry_test

import (
	"context"
	"errors"
	"testing"

	"mgu-showcase/backend/internal/registry"
	"mgu-showcase/backend/internal/registry/memstore"
)

func validItem() registry.Item {
	return registry.Item{
		Slug: "mfk", Name: "МФК", TargetType: registry.TargetExternalURL, Target: "https://mfc.msu.ru/",
		Status: registry.StatusActive, Visibility: registry.VisibilityPublic,
		Owner: registry.Owner{Name: "Команда витрины"},
	}
}

func fields(err error) map[string]bool {
	var ve *registry.ValidationError
	out := map[string]bool{}
	if errors.As(err, &ve) {
		for _, f := range ve.Fields {
			out[f.Field] = true
		}
	}
	return out
}

func TestCreateValidation(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name  string
		mod   func(*registry.Item)
		field string
	}{
		{"no owner", func(it *registry.Item) { it.Owner.Name = " " }, "owner.name"},
		{"bad slug", func(it *registry.Item) { it.Slug = "Bad Slug" }, "slug"},
		{"no name", func(it *registry.Item) { it.Name = "" }, "name"},
		{"unknown target type", func(it *registry.Item) { it.TargetType = "ftp" }, "target_type"},
		{"not yet supported target type", func(it *registry.Item) { it.TargetType = registry.TargetInternalRoute; it.Target = "/profile" }, "target_type"},
		{"relative url", func(it *registry.Item) { it.Target = "/profile" }, "target"},
		{"javascript url", func(it *registry.Item) { it.Target = "javascript:alert(1)" }, "target"},
		{"url with credentials", func(it *registry.Item) { it.Target = "https://u:p@msu.ru" }, "target"},
		{"bad status", func(it *registry.Item) { it.Status = "archived" }, "status"},
		{"role_based without roles", func(it *registry.Item) { it.Visibility = registry.VisibilityRoleBased }, "allowed_roles"},
		{"roles without role_based", func(it *registry.Item) { it.AllowedRoles = []string{"student"} }, "allowed_roles"},
		{"bad role name", func(it *registry.Item) {
			it.Visibility = registry.VisibilityRoleBased
			it.AllowedRoles = []string{"Student Role"}
		}, "allowed_roles"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := registry.New(memstore.New())
			it := validItem()
			tc.mod(&it)
			_, err := reg.Create(ctx, it)
			if !fields(err)[tc.field] {
				t.Fatalf("expected validation error on %q, got %v", tc.field, err)
			}
		})
	}
}

func TestCreateDefaultsAndNormalization(t *testing.T) {
	reg := registry.New(memstore.New())
	it := validItem()
	it.Status, it.Visibility = "", ""
	it.Name = "  МФК  "
	got, err := reg.Create(context.Background(), it)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != registry.StatusDraft || got.Visibility != registry.VisibilityPublic {
		t.Fatalf("defaults: status=%s visibility=%s", got.Status, got.Visibility)
	}
	if got.Name != "МФК" || !registry.IsValidID(got.ID) || got.CreatedAt.IsZero() {
		t.Fatalf("unexpected item: %+v", got)
	}
}

func TestSlugConflict(t *testing.T) {
	ctx := context.Background()
	reg := registry.New(memstore.New())
	if _, err := reg.Create(ctx, validItem()); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Create(ctx, validItem()); !errors.Is(err, registry.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
}

func TestCatalogVisibility(t *testing.T) {
	ctx := context.Background()
	reg := registry.New(memstore.New())
	mk := func(slug string, st registry.Status, vis registry.Visibility, roles ...string) {
		it := validItem()
		it.Slug, it.Name, it.Status, it.Visibility, it.AllowedRoles = slug, slug, st, vis, roles
		if _, err := reg.Create(ctx, it); err != nil {
			t.Fatal(err)
		}
	}
	mk("public", registry.StatusActive, registry.VisibilityPublic)
	mk("maint", registry.StatusMaintenance, registry.VisibilityPublic)
	mk("draft", registry.StatusDraft, registry.VisibilityPublic)
	mk("disabled", registry.StatusDisabled, registry.VisibilityPublic)
	mk("authed", registry.StatusActive, registry.VisibilityAuthenticated)
	mk("staff", registry.StatusActive, registry.VisibilityRoleBased, "staff")

	slugs := func(v registry.Viewer) map[string]bool {
		items, err := reg.Catalog(ctx, v)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]bool{}
		for _, it := range items {
			m[it.Slug] = true
		}
		return m
	}
	check := func(name string, got map[string]bool, want ...string) {
		if len(got) != len(want) {
			t.Fatalf("%s: got %v, want %v", name, got, want)
		}
		for _, w := range want {
			if !got[w] {
				t.Fatalf("%s: missing %s in %v", name, w, got)
			}
		}
	}
	check("anonymous", slugs(registry.Anonymous), "public", "maint")
	check("student", slugs(registry.Viewer{Authenticated: true, Roles: []string{"student"}}), "public", "maint", "authed")
	check("staff", slugs(registry.Viewer{Authenticated: true, Roles: []string{"staff"}}), "public", "maint", "authed", "staff")
	// роли без аутентификации не дают доступа
	check("forged roles", slugs(registry.Viewer{Roles: []string{"staff"}}), "public", "maint")
}

func TestPatch(t *testing.T) {
	ctx := context.Background()
	reg := registry.New(memstore.New())
	it := validItem()
	it.Visibility, it.AllowedRoles = registry.VisibilityRoleBased, []string{"staff", "student", "staff"}
	created, err := reg.Create(ctx, it)
	if err != nil {
		t.Fatal(err)
	}
	if len(created.AllowedRoles) != 2 {
		t.Fatalf("roles not deduplicated: %v", created.AllowedRoles)
	}

	pub := registry.VisibilityPublic
	name := "Новое имя"
	upd, err := reg.Update(ctx, created.ID, registry.Patch{Visibility: &pub, Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if upd.Name != name || len(upd.AllowedRoles) != 0 || upd.Target != created.Target {
		t.Fatalf("unexpected patch result: %+v", upd)
	}

	bad := "not a url"
	if _, err := reg.Update(ctx, created.ID, registry.Patch{Target: &bad}); !fields(err)["target"] {
		t.Fatalf("want target validation error, got %v", err)
	}
	if _, err := reg.Update(ctx, "nope", registry.Patch{}); !errors.Is(err, registry.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestSeedIdempotent(t *testing.T) {
	ctx := context.Background()
	reg := registry.New(memstore.New())
	it := validItem()
	it.Status = ""
	if res, err := reg.Seed(ctx, it, false); err != nil || res != registry.SeedCreated {
		t.Fatalf("first seed: %v %v", res, err)
	}
	it.Name = "Изменено"
	if res, _ := reg.Seed(ctx, it, false); res != registry.SeedSkipped {
		t.Fatalf("want skipped, got %v", res)
	}
	if res, _ := reg.Seed(ctx, it, true); res != registry.SeedUpdated {
		t.Fatalf("want updated, got %v", res)
	}
	items, _ := reg.List(ctx, registry.ListFilter{})
	if len(items) != 1 || items[0].Name != "Изменено" || items[0].Status != registry.StatusActive {
		t.Fatalf("unexpected: %+v", items)
	}
}
