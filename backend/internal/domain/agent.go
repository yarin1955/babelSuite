package domain

import "time"

type Agent struct {
	AgentID           string            `json:"agent_id"          bson:"agent_id"`
	OrgID             string            `json:"org_id"            bson:"org_id"`
	Name              string            `json:"name"              bson:"name"`
	Token             string            `json:"-"                 bson:"token"`
	RuntimeTargetID   string            `json:"runtime_target_id" bson:"runtime_target_id"`
	DesiredBackend    string            `json:"desired_backend"   bson:"desired_backend"`
	DesiredPlatform   string            `json:"desired_platform"  bson:"desired_platform"`
	DesiredTargetName string            `json:"desired_target_name" bson:"desired_target_name"`
	DesiredTargetURL  string            `json:"desired_target_url"  bson:"desired_target_url"`
	Platform          string            `json:"platform"          bson:"platform"`
	Backend           string            `json:"backend"           bson:"backend"`
	TargetName        string            `json:"target_name"       bson:"target_name"`
	TargetURL         string            `json:"target_url"        bson:"target_url"`
	Capacity          int               `json:"capacity"          bson:"capacity"`
	Version           string            `json:"version"           bson:"version"`
	Labels            map[string]string `json:"labels"            bson:"labels"`
	LastContact       time.Time         `json:"last_contact"      bson:"last_contact"`
	LastWork          *time.Time        `json:"last_work"         bson:"last_work,omitempty"`
	NoSchedule        bool              `json:"no_schedule"       bson:"no_schedule"`
	CreatedAt         time.Time         `json:"created_at"        bson:"created_at"`
}
