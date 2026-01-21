package bunadapter

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/goliatone/go-featuregate/gate"
	"github.com/goliatone/go-featuregate/store"
)

// DefaultTable is the default table name for feature flag overrides.
const DefaultTable = "feature_flags"

// ErrDBRequired indicates the underlying Bun DB is missing.
var ErrDBRequired = errors.New("bunadapter: db is required")

// ErrInvalidKey indicates a missing or invalid feature key.
var ErrInvalidKey = errors.New("bunadapter: feature key required")

// Store adapts Bun DB operations to featuregate overrides.
type Store struct {
	db        bun.IDB
	table     string
	now       func() time.Time
	updatedBy func(gate.ActorRef) string
}

// Option customizes the Bun store adapter.
type Option func(*Store)

// NewStore constructs a new Bun-backed override store.
func NewStore(db bun.IDB, opts ...Option) *Store {
	adapter := &Store{
		db:        db,
		table:     DefaultTable,
		now:       time.Now,
		updatedBy: defaultUpdatedBy,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(adapter)
		}
	}
	if adapter.table == "" {
		adapter.table = DefaultTable
	}
	if adapter.now == nil {
		adapter.now = time.Now
	}
	if adapter.updatedBy == nil {
		adapter.updatedBy = defaultUpdatedBy
	}
	return adapter
}

// WithTable sets the table name used for overrides.
func WithTable(table string) Option {
	return func(adapter *Store) {
		if adapter == nil {
			return
		}
		adapter.table = strings.TrimSpace(table)
	}
}

// WithNowFunc overrides the timestamp function used for updates.
func WithNowFunc(now func() time.Time) Option {
	return func(adapter *Store) {
		if adapter == nil {
			return
		}
		adapter.now = now
	}
}

// WithUpdatedByBuilder overrides the updated_by value builder.
func WithUpdatedByBuilder(builder func(gate.ActorRef) string) Option {
	return func(adapter *Store) {
		if adapter == nil {
			return
		}
		adapter.updatedBy = builder
	}
}

// FeatureFlagRecord maps to the feature_flags table.
type FeatureFlagRecord struct {
	bun.BaseModel `bun:"table:feature_flags"`
	Key           string    `bun:"key,pk"`
	ScopeType     string    `bun:"scope_type,pk"`
	ScopeID       string    `bun:"scope_id,pk"`
	Enabled       *bool     `bun:"enabled,nullzero"`
	UpdatedBy     string    `bun:"updated_by,nullzero"`
	UpdatedAt     time.Time `bun:"updated_at,nullzero"`
}

// Get implements store.Reader.
func (s *Store) Get(ctx context.Context, key string, scopeSet gate.ScopeSet) (store.Override, error) {
	if s == nil || s.db == nil {
		return store.MissingOverride(), ErrDBRequired
	}
	normalized, err := normalizeKey(key)
	if err != nil {
		return store.MissingOverride(), err
	}
	for _, scope := range readScopes(scopeSet) {
		record := FeatureFlagRecord{}
		query := s.db.NewSelect().Model(&record).
			Where("key = ?", normalized).
			Where("scope_type = ?", scope.kind).
			Where("scope_id = ?", scope.id).
			Limit(1)
		if s.table != "" {
			query = query.TableExpr(s.table)
		}
		if err := query.Scan(ctx); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return store.MissingOverride(), err
		}
		return overrideFromRecord(record), nil
	}
	return store.MissingOverride(), nil
}

// Set implements store.Writer.
func (s *Store) Set(ctx context.Context, key string, scopeSet gate.ScopeSet, enabled bool, actor gate.ActorRef) error {
	if s == nil || s.db == nil {
		return ErrDBRequired
	}
	normalized, err := normalizeKey(key)
	if err != nil {
		return err
	}
	scope := writeScope(scopeSet)
	return s.upsert(ctx, normalized, scope, boolPtr(enabled), actor)
}

// Unset implements store.Writer.
func (s *Store) Unset(ctx context.Context, key string, scopeSet gate.ScopeSet, actor gate.ActorRef) error {
	if s == nil || s.db == nil {
		return ErrDBRequired
	}
	normalized, err := normalizeKey(key)
	if err != nil {
		return err
	}
	scope := writeScope(scopeSet)
	return s.upsert(ctx, normalized, scope, nil, actor)
}

// Delete removes a stored override row.
func (s *Store) Delete(ctx context.Context, key string, scopeSet gate.ScopeSet) error {
	if s == nil || s.db == nil {
		return ErrDBRequired
	}
	normalized, err := normalizeKey(key)
	if err != nil {
		return err
	}
	scope := writeScope(scopeSet)
	query := s.db.NewDelete().
		Where("key = ?", normalized).
		Where("scope_type = ?", scope.kind).
		Where("scope_id = ?", scope.id)
	if s.table != "" {
		query = query.TableExpr(s.table)
	}
	_, err = query.Exec(ctx)
	return err
}

func (s *Store) upsert(ctx context.Context, key string, scope scopeKey, enabled *bool, actor gate.ActorRef) error {
	record := FeatureFlagRecord{
		Key:       key,
		ScopeType: string(scope.kind),
		ScopeID:   scope.id,
		Enabled:   enabled,
		UpdatedBy: s.updatedBy(actor),
		UpdatedAt: s.now(),
	}
	query := s.db.NewInsert().Model(&record).
		On("CONFLICT (key, scope_type, scope_id) DO UPDATE").
		Set("enabled = EXCLUDED.enabled").
		Set("updated_by = EXCLUDED.updated_by").
		Set("updated_at = EXCLUDED.updated_at")
	if s.table != "" {
		query = query.TableExpr(s.table)
	}
	_, err := query.Exec(ctx)
	return err
}

func defaultUpdatedBy(actor gate.ActorRef) string {
	if actor.ID != "" {
		return actor.ID
	}
	if actor.Name != "" {
		return actor.Name
	}
	if actor.Type != "" {
		return actor.Type
	}
	return ""
}

func normalizeKey(key string) (string, error) {
	normalized := gate.NormalizeKey(strings.TrimSpace(key))
	if normalized == "" {
		return "", ErrInvalidKey
	}
	return normalized, nil
}

func boolPtr(value bool) *bool {
	return &value
}

type scopeKey struct {
	kind scopeKind
	id   string
}

type scopeKind string

const (
	scopeSystem scopeKind = "system"
	scopeTenant scopeKind = "tenant"
	scopeOrg    scopeKind = "org"
	scopeUser   scopeKind = "user"
)

func readScopes(scopeSet gate.ScopeSet) []scopeKey {
	scopes := make([]scopeKey, 0, 4)
	if scopeSet.UserID != "" {
		scopes = append(scopes, scopeKey{kind: scopeUser, id: scopeSet.UserID})
	}
	if scopeSet.OrgID != "" {
		scopes = append(scopes, scopeKey{kind: scopeOrg, id: scopeSet.OrgID})
	}
	if scopeSet.TenantID != "" {
		scopes = append(scopes, scopeKey{kind: scopeTenant, id: scopeSet.TenantID})
	}
	scopes = append(scopes, scopeKey{kind: scopeSystem})
	return scopes
}

func writeScope(scopeSet gate.ScopeSet) scopeKey {
	switch {
	case scopeSet.UserID != "":
		return scopeKey{kind: scopeUser, id: scopeSet.UserID}
	case scopeSet.OrgID != "":
		return scopeKey{kind: scopeOrg, id: scopeSet.OrgID}
	case scopeSet.TenantID != "":
		return scopeKey{kind: scopeTenant, id: scopeSet.TenantID}
	default:
		return scopeKey{kind: scopeSystem}
	}
}

func overrideFromRecord(record FeatureFlagRecord) store.Override {
	if record.Enabled == nil {
		return store.UnsetOverride()
	}
	if *record.Enabled {
		return store.EnabledOverride()
	}
	return store.DisabledOverride()
}

var _ store.ReadWriter = (*Store)(nil)
