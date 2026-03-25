package domain

import "time"

type ProfileFormat string

const (
	ProfileFormatYAML ProfileFormat = "yaml"
	ProfileFormatJSON ProfileFormat = "json"
)

type Profile struct {
	ProfileID     string        `json:"profile_id"       bson:"profile_id"`
	OrgID         string        `json:"org_id"           bson:"org_id"`
	Name          string        `json:"name"             bson:"name"`
	Description   string        `json:"description"      bson:"description"`
	Format        ProfileFormat `json:"format"           bson:"format"`
	Content       string        `json:"content"          bson:"content"`
	Revision      int           `json:"revision"         bson:"revision"`
	CreatedBy     string        `json:"created_by"       bson:"created_by"`
	CreatedByName string        `json:"created_by_name"  bson:"created_by_name"`
	UpdatedBy     string        `json:"updated_by"       bson:"updated_by"`
	UpdatedByName string        `json:"updated_by_name"  bson:"updated_by_name"`
	CreatedAt     time.Time     `json:"created_at"       bson:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"       bson:"updated_at"`
}
