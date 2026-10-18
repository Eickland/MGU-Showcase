-- Реестр элементов витрины.
CREATE TABLE registry_items (
    id            UUID        PRIMARY KEY,
    slug          TEXT        NOT NULL UNIQUE,
    name          TEXT        NOT NULL,
    description   TEXT        NOT NULL DEFAULT '',
    icon          TEXT        NOT NULL DEFAULT '',

    -- Все предусмотренные моделью типы; какие из них реально поддержаны, решает приложение.
    target_type   TEXT        NOT NULL
        CHECK (target_type IN ('external_url', 'internal_route', 'application', 'embedded')),
    target        TEXT        NOT NULL,

    status        TEXT        NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'active', 'maintenance', 'disabled')),
    visibility    TEXT        NOT NULL DEFAULT 'public'
        CHECK (visibility IN ('public', 'authenticated', 'role_based')),
    allowed_roles TEXT[]      NOT NULL DEFAULT '{}',

    owner_name    TEXT        NOT NULL CHECK (owner_name <> ''),
    owner_contact TEXT        NOT NULL DEFAULT '',

    sort_order    INTEGER     NOT NULL DEFAULT 0,
    pinned        BOOLEAN     NOT NULL DEFAULT FALSE,

    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT role_based_needs_roles CHECK (
        (visibility = 'role_based' AND cardinality(allowed_roles) > 0)
        OR (visibility <> 'role_based' AND cardinality(allowed_roles) = 0)
    )
);

CREATE INDEX registry_items_catalog_idx ON registry_items (status, pinned DESC, sort_order, name);
