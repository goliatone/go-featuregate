-- Feature flag overrides (nullable enabled means explicit unset).
-- scope_type values: system, tenant, org, user.
CREATE TABLE feature_flags (
    key text NOT NULL,
    scope_type text NOT NULL,
    scope_id text NOT NULL DEFAULT '',
    enabled boolean NULL,
    updated_by text,
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    PRIMARY KEY (key, scope_type, scope_id)
);
