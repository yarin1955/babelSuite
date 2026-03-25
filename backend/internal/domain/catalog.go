package domain

import "time"

type RegistryKind string

const (
	RegistryGHCR  RegistryKind = "ghcr"
	RegistryJFrog RegistryKind = "jfrog"
)

type Registry struct {
	RegistryID            string       `json:"registry_id" bson:"registry_id"`
	OrgID                 string       `json:"org_id"      bson:"org_id"`
	Kind                  RegistryKind `json:"kind"        bson:"kind"`
	Name                  string       `json:"name"        bson:"name"`
	URL                   string       `json:"url"         bson:"url"`
	InsecureSkipTLSVerify bool         `json:"-"           bson:"insecure_skip_tls_verify"`
	Username              string       `json:"-"           bson:"username"`
	Token                 string       `json:"-"           bson:"token"`
	Password              string       `json:"-"           bson:"password"`
	TLSCAData             string       `json:"-"           bson:"tls_ca_data"`
	TLSCertData           string       `json:"-"           bson:"tls_cert_data"`
	TLSKeyData            string       `json:"-"           bson:"tls_key_data"`
	Enabled               bool         `json:"enabled"     bson:"enabled"`
	CreatedAt             time.Time    `json:"created_at"  bson:"created_at"`
}

type CatalogPackage struct {
	PackageID      string       `json:"package_id"    bson:"package_id"`
	OrgID          string       `json:"org_id"        bson:"org_id"`
	RegistryID     string       `json:"registry_id"   bson:"registry_id"`
	RegistryKind   RegistryKind `json:"registry_kind" bson:"registry_kind"`
	Name           string       `json:"name"          bson:"name"`
	DisplayName    string       `json:"display_name"  bson:"display_name"`
	Description    string       `json:"description"   bson:"description"`
	Publisher      string       `json:"publisher"     bson:"publisher"`
	ImageRef       string       `json:"image_ref"     bson:"image_ref"`
	Version        string       `json:"version"       bson:"version"`
	Tags           []string     `json:"tags"          bson:"tags"`
	Profiles       []string     `json:"profiles,omitempty"        bson:"profiles,omitempty"`
	DefaultProfile string       `json:"default_profile,omitempty" bson:"default_profile,omitempty"`
	ServiceCount   int          `json:"service_count,omitempty"    bson:"service_count,omitempty"`
	MockCount      int          `json:"mock_count,omitempty"       bson:"mock_count,omitempty"`
	TestCount      int          `json:"test_count,omitempty"       bson:"test_count,omitempty"`
	ContractCount  int          `json:"contract_count,omitempty"   bson:"contract_count,omitempty"`
	Enabled        bool         `json:"enabled"       bson:"enabled"`
	UpdatedAt      time.Time    `json:"updated_at"    bson:"updated_at"`
}

type CatalogFilter struct {
	Search       string
	RegistryKind RegistryKind
	EnabledOnly  bool
	Page         int
	PageSize     int
}
