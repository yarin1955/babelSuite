package domain

import "time"

type RuntimeTarget struct {
	RuntimeTargetID       string            `json:"runtime_target_id"         bson:"runtime_target_id"`
	OrgID                 string            `json:"org_id"                    bson:"org_id"`
	Name                  string            `json:"name"                      bson:"name"`
	Backend               string            `json:"backend"                   bson:"backend"`
	Platform              string            `json:"platform"                  bson:"platform"`
	EndpointURL           string            `json:"endpoint_url"              bson:"endpoint_url"`
	Namespace             string            `json:"namespace"                 bson:"namespace"`
	InsecureSkipTLSVerify bool              `json:"-"                         bson:"insecure_skip_tls_verify"`
	Username              string            `json:"-"                         bson:"username"`
	Password              string            `json:"-"                         bson:"password"`
	BearerToken           string            `json:"-"                         bson:"bearer_token"`
	TLSCAData             string            `json:"-"                         bson:"tls_ca_data"`
	TLSCertData           string            `json:"-"                         bson:"tls_cert_data"`
	TLSKeyData            string            `json:"-"                         bson:"tls_key_data"`
	Labels                map[string]string `json:"labels"                    bson:"labels"`
	CreatedAt             time.Time         `json:"created_at"                bson:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"                bson:"updated_at"`
}
