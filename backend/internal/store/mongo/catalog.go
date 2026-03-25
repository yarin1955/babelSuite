package mongo

import (
	"context"
	"math"

	"github.com/babelsuite/babelsuite/internal/domain"
	"github.com/babelsuite/babelsuite/internal/store"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ── Registries ────────────────────────────────────────────────────────────────

func (s *Store) CreateRegistry(ctx context.Context, r *domain.Registry) error {
	_, err := s.registries.InsertOne(ctx, r)
	return wrap(err)
}

func (s *Store) ListRegistries(ctx context.Context, orgID string) ([]*domain.Registry, error) {
	cur, err := s.registries.Find(ctx, bson.M{"org_id": orgID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, wrap(err)
	}
	var out []*domain.Registry
	return out, cur.All(ctx, &out)
}

func (s *Store) GetRegistry(ctx context.Context, id string) (*domain.Registry, error) {
	var r domain.Registry
	return &r, wrap(s.registries.FindOne(ctx, bson.M{"registry_id": id}).Decode(&r))
}

func (s *Store) UpdateRegistry(ctx context.Context, r *domain.Registry) error {
	_, err := s.registries.ReplaceOne(ctx, bson.M{"registry_id": r.RegistryID}, r)
	return wrap(err)
}

func (s *Store) DeleteRegistry(ctx context.Context, id string) error {
	_, err := s.registries.DeleteOne(ctx, bson.M{"registry_id": id})
	return wrap(err)
}

// ── Catalog packages ──────────────────────────────────────────────────────────

func (s *Store) UpsertPackage(ctx context.Context, p *domain.CatalogPackage) error {
	_, err := s.packages.ReplaceOne(ctx,
		bson.M{"org_id": p.OrgID, "image_ref": p.ImageRef},
		p,
		options.Replace().SetUpsert(true),
	)
	return wrap(err)
}

func (s *Store) ListPackages(ctx context.Context, orgID string, f domain.CatalogFilter) ([]*domain.CatalogPackage, int64, error) {
	q := bson.M{"org_id": orgID}
	if f.EnabledOnly {
		q["enabled"] = true
	}
	if f.RegistryKind != "" {
		q["registry_kind"] = f.RegistryKind
	}
	if f.Search != "" {
		q["$or"] = bson.A{
			bson.M{"name": bson.M{"$regex": f.Search, "$options": "i"}},
			bson.M{"display_name": bson.M{"$regex": f.Search, "$options": "i"}},
			bson.M{"description": bson.M{"$regex": f.Search, "$options": "i"}},
			bson.M{"publisher": bson.M{"$regex": f.Search, "$options": "i"}},
			bson.M{"tags": bson.M{"$regex": f.Search, "$options": "i"}},
			bson.M{"profiles": bson.M{"$regex": f.Search, "$options": "i"}},
		}
	}

	total, err := s.packages.CountDocuments(ctx, q)
	if err != nil {
		return nil, 0, wrap(err)
	}

	pageSize := f.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	page := f.Page
	if page <= 0 {
		page = 1
	}
	skip := int64((page - 1) * pageSize)

	opts := options.Find().
		SetSort(bson.D{{Key: "updated_at", Value: -1}}).
		SetSkip(skip).
		SetLimit(int64(pageSize))

	cur, err := s.packages.Find(ctx, q, opts)
	if err != nil {
		return nil, 0, wrap(err)
	}
	var out []*domain.CatalogPackage
	if err := cur.All(ctx, &out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (s *Store) GetPackage(ctx context.Context, id string) (*domain.CatalogPackage, error) {
	var p domain.CatalogPackage
	err := s.packages.FindOne(ctx, bson.M{"package_id": id}).Decode(&p)
	return &p, wrap(err)
}

func (s *Store) SetPackageEnabled(ctx context.Context, id string, enabled bool) error {
	_, err := s.packages.UpdateOne(ctx,
		bson.M{"package_id": id},
		bson.M{"$set": bson.M{"enabled": enabled}},
	)
	return wrap(err)
}

func (s *Store) DeletePackage(ctx context.Context, id string) error {
	_, err := s.packages.DeleteOne(ctx, bson.M{"package_id": id})
	return wrap(err)
}

// totalPages helper used by the handler.
func TotalPages(total int64, pageSize int) int {
	if pageSize <= 0 {
		return 1
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}

// ensure store.Store is satisfied — compile-time check lives in store.go
var _ store.Store = (*Store)(nil)
