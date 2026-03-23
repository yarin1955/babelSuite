package domain

import "time"

type RegistryKind string

const (
	RegistryGHCR  RegistryKind = "ghcr"
	RegistryJFrog RegistryKind = "jfrog"
)

type Registry struct {
	RegistryID string       `json:"registry_id" bson:"registry_id"`
	OrgID      string       `json:"org_id"      bson:"org_id"`
	Kind       RegistryKind `json:"kind"        bson:"kind"`
	Name       string       `json:"name"        bson:"name"`
	URL        string       `json:"url"         bson:"url"`
	Token      string       `json:"token,omitempty" bson:"token"`
	Enabled    bool         `json:"enabled"     bson:"enabled"`
	CreatedAt  time.Time    `json:"created_at"  bson:"created_at"`
}

type CatalogPackage struct {
	PackageID    string       `json:"package_id"    bson:"package_id"`
	OrgID        string       `json:"org_id"        bson:"org_id"`
	RegistryID   string       `json:"registry_id"   bson:"registry_id"`
	RegistryKind RegistryKind `json:"registry_kind" bson:"registry_kind"`
	Name         string       `json:"name"          bson:"name"`
	DisplayName  string       `json:"display_name"  bson:"display_name"`
	Description  string       `json:"description"   bson:"description"`
	Publisher    string       `json:"publisher"     bson:"publisher"`
	ImageRef     string       `json:"image_ref"     bson:"image_ref"`
	Version      string       `json:"version"       bson:"version"`
	Tags         []string     `json:"tags"          bson:"tags"`
	Enabled      bool         `json:"enabled"       bson:"enabled"`
	UpdatedAt    time.Time    `json:"updated_at"    bson:"updated_at"`
}

type CatalogFilter struct {
	Search       string
	RegistryKind RegistryKind
	EnabledOnly  bool
	Page         int
	PageSize     int
}
