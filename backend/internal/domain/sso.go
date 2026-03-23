package domain

import "time"

type OIDCProvider struct {
	ProviderID   string    `json:"provider_id"   bson:"provider_id"`
	Name         string    `json:"name"          bson:"name"`
	IssuerURL    string    `json:"issuer_url"    bson:"issuer_url"`
	ClientID     string    `json:"client_id"     bson:"client_id"`
	ClientSecret string    `json:"-"             bson:"client_secret"`
	Scopes       []string  `json:"scopes"        bson:"scopes"`
	Enabled      bool      `json:"enabled"       bson:"enabled"`
	CreatedAt    time.Time `json:"created_at"    bson:"created_at"`
}
