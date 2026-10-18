// Package httpapi — HTTP-слой: маршруты /api/v1/*, DTO, middleware.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"mgu-showcase/backend/internal/auth"
	"mgu-showcase/backend/internal/registry"
)

type Config struct {
	CORSAllowedOrigins []string
}

type Server struct {
	cfg   Config
	reg   *registry.Registry
	authn auth.Chain
	log   *slog.Logger
}

func NewServer(cfg Config, reg *registry.Registry, authn auth.Chain, log *slog.Logger) *Server {
	return &Server{cfg: cfg, reg: reg, authn: authn, log: log}
}

// Handler собирает маршруты и middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/healthz", s.healthz)
	mux.HandleFunc("GET /api/readyz", s.readyz)

	mux.HandleFunc("GET /api/v1/services", s.listServices)
	mux.HandleFunc("GET /api/v1/services/{id}", s.getService)
	mux.HandleFunc("POST /api/v1/services", requireAdmin(s.createService))
	mux.HandleFunc("PATCH /api/v1/services/{id}", requireAdmin(s.updateService))
	mux.HandleFunc("DELETE /api/v1/services/{id}", requireAdmin(s.deleteService))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, http.StatusNotFound, "not_found", "Маршрут не найден")
	})

	var h http.Handler = mux
	h = s.withAuth(h)
	h = s.withCORS(h)
	h = s.withLogging(h)
	h = s.withRecover(h)
	h = withRequestID(h)
	return h
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.reg.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func viewerFrom(p auth.Principal) registry.Viewer {
	return registry.Viewer{Authenticated: p.Authenticated, Roles: p.Roles}
}

// scopeAll — запрошено ли полное представление (?scope=all). Только для админов.
func scopeAll(w http.ResponseWriter, r *http.Request) (all, ok bool) {
	switch r.URL.Query().Get("scope") {
	case "", "catalog":
		return false, true
	case "all":
		p := auth.FromContext(r.Context())
		if !p.Authenticated {
			writeError(w, r, http.StatusUnauthorized, "unauthorized", "Требуется авторизация")
			return false, false
		}
		if !p.Admin {
			writeError(w, r, http.StatusForbidden, "forbidden", "Недостаточно прав")
			return false, false
		}
		return true, true
	default:
		writeError(w, r, http.StatusBadRequest, "bad_request", "scope: допустимые значения catalog, all")
		return false, false
	}
}

// GET /api/v1/services — витрина для текущего пользователя.
// GET /api/v1/services?scope=all[&status=draft,active] — весь реестр (админ).
func (s *Server) listServices(w http.ResponseWriter, r *http.Request) {
	all, ok := scopeAll(w, r)
	if !ok {
		return
	}
	if !all {
		items, err := s.reg.Catalog(r.Context(), viewerFrom(auth.FromContext(r.Context())))
		if err != nil {
			s.writeDomainError(w, r, err)
			return
		}
		out := make([]catalogItem, len(items))
		for i, it := range items {
			out[i] = toCatalog(it)
		}
		writeJSON(w, http.StatusOK, listResponse[catalogItem]{Items: out})
		return
	}

	var f registry.ListFilter
	for _, st := range splitCSV(r.URL.Query().Get("status")) {
		if !registry.Status(st).Valid() {
			writeError(w, r, http.StatusBadRequest, "bad_request", "status: неизвестный статус "+st)
			return
		}
		f.Statuses = append(f.Statuses, registry.Status(st))
	}
	items, err := s.reg.List(r.Context(), f)
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	out := make([]fullItem, len(items))
	for i, it := range items {
		out[i] = toFull(it)
	}
	writeJSON(w, http.StatusOK, listResponse[fullItem]{Items: out})
}

func (s *Server) getService(w http.ResponseWriter, r *http.Request) {
	all, ok := scopeAll(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if all {
		it, err := s.reg.Get(r.Context(), id)
		if err != nil {
			s.writeDomainError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, toFull(it))
		return
	}
	it, err := s.reg.CatalogItem(r.Context(), id, viewerFrom(auth.FromContext(r.Context())))
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toCatalog(it))
}

func (s *Server) createService(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	it, err := s.reg.Create(r.Context(), req.item())
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/services/"+it.ID)
	writeJSON(w, http.StatusCreated, toFull(it))
}

func (s *Server) updateService(w http.ResponseWriter, r *http.Request) {
	var req patchRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	it, err := s.reg.Update(r.Context(), r.PathValue("id"), req.patch())
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toFull(it))
}

func (s *Server) deleteService(w http.ResponseWriter, r *http.Request) {
	if err := s.reg.Delete(r.Context(), r.PathValue("id")); err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
