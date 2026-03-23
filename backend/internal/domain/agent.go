package domain

import "time"

type Agent struct {
	AgentID     string            `json:"agent_id"     bson:"agent_id"`
	OrgID       string            `json:"org_id"       bson:"org_id"`
	Name        string            `json:"name"         bson:"name"`
	Token       string            `json:"-"            bson:"token"`
	Platform    string            `json:"platform"     bson:"platform"`
	Backend     string            `json:"backend"      bson:"backend"`
	Capacity    int               `json:"capacity"     bson:"capacity"`
	Version     string            `json:"version"      bson:"version"`
	Labels      map[string]string `json:"labels"       bson:"labels"`
	LastContact time.Time         `json:"last_contact" bson:"last_contact"`
	NoSchedule  bool              `json:"no_schedule"  bson:"no_schedule"`
	CreatedAt   time.Time         `json:"created_at"   bson:"created_at"`
}
