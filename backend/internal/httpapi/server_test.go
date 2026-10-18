package httpapi_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mgu-showcase/backend/internal/auth"
	"mgu-showcase/backend/internal/httpapi"
	"mgu-showcase/backend/internal/registry"
	"mgu-showcase/backend/internal/registry/memstore"
)

const adminToken = "test-admin-token-123456"

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	reg := registry.New(memstore.New())
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	authn := auth.Chain{auth.AdminToken{Token: adminToken}, auth.DevHeaders{}}
	srv := httptest.NewServer(httpapi.NewServer(httpapi.Config{CORSAllowedOrigins: []string{"http://localhost:5173"}}, reg, authn, log).Handler())
	t.Cleanup(srv.Close)
	return srv
}

type req struct {
	method, path, body string
	headers            map[string]string
}

func do(t *testing.T, srv *httptest.Server, r req) (int, map[string]any) {
	t.Helper()
	var body io.Reader
	if r.body != "" {
		body = strings.NewReader(r.body)
	}
	hr, _ := http.NewRequest(r.method, srv.URL+r.path, body)
	for k, v := range r.headers {
		hr.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(hr)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("non-JSON response %d: %s", resp.StatusCode, raw)
		}
	}
	return resp.StatusCode, out
}

var admin = map[string]string{"Authorization": "Bearer " + adminToken}

const createBody = `{
	"slug": "mfk", "name": "МФК", "description": "Межфакультетские курсы", "icon": "✏️",
	"target_type": "external_url", "target": "https://mfc.msu.ru/",
	"status": "active", "visibility": "public",
	"owner": {"name": "Команда витрины", "contact": "showcase@msu.ru"}
}`

func TestCRUDFlow(t *testing.T) {
	srv := newTestServer(t)

	// без токена создавать нельзя
	if code, _ := do(t, srv, req{method: "POST", path: "/api/v1/services", body: createBody}); code != 401 {
		t.Fatalf("anonymous create: want 401, got %d", code)
	}
	// обычный пользователь — 403
	if code, _ := do(t, srv, req{method: "POST", path: "/api/v1/services", body: createBody,
		headers: map[string]string{"X-Dev-User": "ivan"}}); code != 403 {
		t.Fatalf("non-admin create: want 403, got %d", code)
	}
	// неверный токен — 401
	if code, _ := do(t, srv, req{method: "GET", path: "/api/v1/services",
		headers: map[string]string{"Authorization": "Bearer wrong"}}); code != 401 {
		t.Fatalf("bad token: want 401, got %d", code)
	}

	code, created := do(t, srv, req{method: "POST", path: "/api/v1/services", body: createBody, headers: admin})
	if code != 201 {
		t.Fatalf("create: %d %v", code, created)
	}
	id := created["id"].(string)
	if created["owner"].(map[string]any)["name"] != "Команда витрины" {
		t.Fatalf("owner missing in admin view: %v", created)
	}

	// публичный список: без владельца и правил доступа
	code, list := do(t, srv, req{method: "GET", path: "/api/v1/services"})
	items := list["items"].([]any)
	if code != 200 || len(items) != 1 {
		t.Fatalf("catalog: %d %v", code, list)
	}
	item := items[0].(map[string]any)
	for _, hidden := range []string{"owner", "allowed_roles", "visibility", "sort_order"} {
		if _, ok := item[hidden]; ok {
			t.Fatalf("catalog must not expose %q: %v", hidden, item)
		}
	}
	if item["target_type"] != "external_url" || item["target"] != "https://mfc.msu.ru/" {
		t.Fatalf("unexpected catalog item: %v", item)
	}

	// scope=all — только админ
	if code, _ := do(t, srv, req{method: "GET", path: "/api/v1/services?scope=all"}); code != 401 {
		t.Fatalf("scope=all anonymous: want 401, got %d", code)
	}

	// PATCH: перевод в role_based скрывает от анонима
	code, patched := do(t, srv, req{method: "PATCH", path: "/api/v1/services/" + id, headers: admin,
		body: `{"visibility":"role_based","allowed_roles":["staff"]}`})
	if code != 200 || patched["name"] != "МФК" {
		t.Fatalf("patch: %d %v", code, patched)
	}
	if code, _ := do(t, srv, req{method: "GET", path: "/api/v1/services/" + id}); code != 404 {
		t.Fatalf("hidden item for anonymous: want 404, got %d", code)
	}
	if code, _ := do(t, srv, req{method: "GET", path: "/api/v1/services/" + id,
		headers: map[string]string{"X-Dev-User": "petrov", "X-Dev-Roles": "staff"}}); code != 200 {
		t.Fatalf("staff should see item, got %d", code)
	}
	if code, l := do(t, srv, req{method: "GET", path: "/api/v1/services?scope=all&status=active", headers: admin}); code != 200 || len(l["items"].([]any)) != 1 {
		t.Fatalf("admin list: %d %v", code, l)
	}

	// валидация
	code, verr := do(t, srv, req{method: "PATCH", path: "/api/v1/services/" + id, headers: admin,
		body: `{"target":"javascript:alert(1)"}`})
	if code != 422 || verr["error"].(map[string]any)["code"] != "validation_failed" {
		t.Fatalf("validation: %d %v", code, verr)
	}
	// неизвестное поле
	if code, _ := do(t, srv, req{method: "PATCH", path: "/api/v1/services/" + id, headers: admin,
		body: `{"titel":"x"}`}); code != 400 {
		t.Fatalf("unknown field: want 400, got %d", code)
	}
	// дубликат slug
	if code, _ := do(t, srv, req{method: "POST", path: "/api/v1/services", body: createBody, headers: admin}); code != 409 {
		t.Fatalf("duplicate: want 409, got %d", code)
	}

	if code, _ := do(t, srv, req{method: "DELETE", path: "/api/v1/services/" + id, headers: admin}); code != 204 {
		t.Fatalf("delete: want 204, got %d", code)
	}
	if code, _ := do(t, srv, req{method: "DELETE", path: "/api/v1/services/" + id, headers: admin}); code != 404 {
		t.Fatalf("delete again: want 404, got %d", code)
	}
}

func TestCORSAndHealth(t *testing.T) {
	srv := newTestServer(t)
	hr, _ := http.NewRequest("OPTIONS", srv.URL+"/api/v1/services", nil)
	hr.Header.Set("Origin", "http://localhost:5173")
	hr.Header.Set("Access-Control-Request-Method", "PATCH")
	resp, err := http.DefaultClient.Do(hr)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 204 || resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("preflight: %d %v", resp.StatusCode, resp.Header)
	}
	if code, body := do(t, srv, req{method: "GET", path: "/api/healthz"}); code != 200 || body["status"] != "ok" {
		t.Fatalf("healthz: %d %v", code, body)
	}
	if code, _ := do(t, srv, req{method: "GET", path: "/nope"}); code != 404 {
		t.Fatalf("unknown route: %d", code)
	}
}
