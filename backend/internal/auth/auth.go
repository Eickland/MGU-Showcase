// Package auth — минимальная заглушка идентификации.
//
// Полноценного RBAC/Keycloak пока нет. Пакет определяет точку расширения
// (Authenticator), а сейчас умеет два простых способа:
//   - статический админский токен (Authorization: Bearer <ADMIN_TOKEN>) — для CRUD;
//   - dev-заголовки X-Dev-User / X-Dev-Roles — только для локальной разработки,
//     чтобы проверять authenticated/role_based видимость.
//
// Позже сюда добавляется, например, KeycloakAuthenticator, проверяющий JWT.
package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
)

// Principal — кто делает запрос.
type Principal struct {
	Subject       string
	Authenticated bool
	Roles         []string
	Admin         bool // право управлять реестром
}

// ErrInvalidCredentials — учётные данные переданы, но не приняты ни одним способом.
var ErrInvalidCredentials = errors.New("auth: invalid credentials")

// Authenticator пытается опознать запрос.
// ok=false означает «это не мой способ», и управление передаётся следующему.
type Authenticator interface {
	Authenticate(r *http.Request) (p Principal, ok bool, err error)
}

// Chain перебирает способы по порядку. Если ни один не подошёл, но заголовок
// Authorization передан, — это ошибка (не молча считаем анонимом).
type Chain []Authenticator

func (c Chain) Authenticate(r *http.Request) (Principal, error) {
	for _, a := range c {
		p, ok, err := a.Authenticate(r)
		if err != nil {
			return Principal{}, err
		}
		if ok {
			return p, nil
		}
	}
	if r.Header.Get("Authorization") != "" {
		return Principal{}, ErrInvalidCredentials
	}
	return Principal{}, nil
}

// AdminToken принимает Bearer-токен, совпадающий с заданным.
type AdminToken struct{ Token string }

func (a AdminToken) Authenticate(r *http.Request) (Principal, bool, error) {
	if a.Token == "" {
		return Principal{}, false, nil
	}
	tok, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !found || subtle.ConstantTimeCompare([]byte(strings.TrimSpace(tok)), []byte(a.Token)) != 1 {
		return Principal{}, false, nil
	}
	return Principal{Subject: "admin-token", Authenticated: true, Admin: true}, true, nil
}

// DevHeaders — ТОЛЬКО для разработки: доверяет заголовкам клиента.
type DevHeaders struct{}

func (DevHeaders) Authenticate(r *http.Request) (Principal, bool, error) {
	user := strings.TrimSpace(r.Header.Get("X-Dev-User"))
	if user == "" {
		return Principal{}, false, nil
	}
	var roles []string
	for _, x := range strings.Split(r.Header.Get("X-Dev-Roles"), ",") {
		if x = strings.TrimSpace(x); x != "" {
			roles = append(roles, x)
		}
	}
	return Principal{Subject: user, Authenticated: true, Roles: roles}, true, nil
}

type ctxKey struct{}

// WithPrincipal кладёт Principal в контекст.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// FromContext достаёт Principal (аноним, если его нет).
func FromContext(ctx context.Context) Principal {
	p, _ := ctx.Value(ctxKey{}).(Principal)
	return p
}
