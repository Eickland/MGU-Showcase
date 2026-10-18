// Package pgstore — хранилище реестра в PostgreSQL.
package pgstore

import (
	"context"
	"database/sql"
	"errors"

	"github.com/lib/pq"

	"mgu-showcase/backend/internal/registry"
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store { return &Store{db: db} }

var _ registry.Repository = (*Store)(nil)

const columns = `id, slug, name, description, icon, target_type, target, status, visibility,
	allowed_roles, owner_name, owner_contact, sort_order, pinned, created_at, updated_at`

const orderBy = ` ORDER BY pinned DESC, sort_order ASC, name ASC, id ASC`

type scanner interface{ Scan(dest ...any) error }

func scanItem(row scanner) (registry.Item, error) {
	var it registry.Item
	var roles pq.StringArray
	err := row.Scan(&it.ID, &it.Slug, &it.Name, &it.Description, &it.Icon, &it.TargetType, &it.Target,
		&it.Status, &it.Visibility, &roles, &it.Owner.Name, &it.Owner.Contact, &it.SortOrder, &it.Pinned,
		&it.CreatedAt, &it.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return it, registry.ErrNotFound
	}
	it.AllowedRoles = []string(roles)
	it.CreatedAt, it.UpdatedAt = it.CreatedAt.UTC(), it.UpdatedAt.UTC()
	return it, err
}

func (s *Store) List(ctx context.Context, f registry.ListFilter) ([]registry.Item, error) {
	q := `SELECT ` + columns + ` FROM registry_items`
	var args []any
	if len(f.Statuses) > 0 {
		st := make([]string, len(f.Statuses))
		for i, v := range f.Statuses {
			st[i] = string(v)
		}
		q += ` WHERE status = ANY($1)`
		args = append(args, pq.Array(st))
	}
	rows, err := s.db.QueryContext(ctx, q+orderBy, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []registry.Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) Get(ctx context.Context, id string) (registry.Item, error) {
	return scanItem(s.db.QueryRowContext(ctx, `SELECT `+columns+` FROM registry_items WHERE id = $1`, id))
}

func (s *Store) GetBySlug(ctx context.Context, slug string) (registry.Item, error) {
	return scanItem(s.db.QueryRowContext(ctx, `SELECT `+columns+` FROM registry_items WHERE slug = $1`, slug))
}

func roles(it registry.Item) any {
	if it.AllowedRoles == nil {
		return pq.Array([]string{})
	}
	return pq.Array(it.AllowedRoles)
}

func (s *Store) Create(ctx context.Context, it registry.Item) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO registry_items (`+columns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		it.ID, it.Slug, it.Name, it.Description, it.Icon, it.TargetType, it.Target, it.Status, it.Visibility,
		roles(it), it.Owner.Name, it.Owner.Contact, it.SortOrder, it.Pinned, it.CreatedAt, it.UpdatedAt)
	return mapErr(err)
}

func (s *Store) Update(ctx context.Context, it registry.Item) error {
	res, err := s.db.ExecContext(ctx, `UPDATE registry_items SET
		slug=$2, name=$3, description=$4, icon=$5, target_type=$6, target=$7, status=$8, visibility=$9,
		allowed_roles=$10, owner_name=$11, owner_contact=$12, sort_order=$13, pinned=$14, updated_at=$15
		WHERE id=$1`,
		it.ID, it.Slug, it.Name, it.Description, it.Icon, it.TargetType, it.Target, it.Status, it.Visibility,
		roles(it), it.Owner.Name, it.Owner.Contact, it.SortOrder, it.Pinned, it.UpdatedAt)
	if err != nil {
		return mapErr(err)
	}
	return expectOne(res)
}

func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM registry_items WHERE id = $1`, id)
	if err != nil {
		return err
	}
	return expectOne(res)
}

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func expectOne(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return registry.ErrNotFound
	}
	return nil
}

func mapErr(err error) error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" { // unique_violation
		return registry.ErrConflict
	}
	return err
}
