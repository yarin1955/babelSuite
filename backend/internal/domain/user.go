package domain

import "time"

type User struct {
	UserID    string    `json:"user_id"    bson:"user_id"`
	OrgID     string    `json:"org_id"     bson:"org_id"`
	Username  string    `json:"username"   bson:"username"`
	Email     string    `json:"email"      bson:"email"`
	Name      string    `json:"name"       bson:"name"`
	IsAdmin   bool      `json:"is_admin"   bson:"is_admin"`
	PassHash  string    `json:"-"          bson:"pass_hash"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}

type Org struct {
	OrgID     string    `json:"org_id"     bson:"org_id"`
	Slug      string    `json:"slug"       bson:"slug"`
	Name      string    `json:"name"       bson:"name"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}
