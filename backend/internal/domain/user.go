package domain

import "time"

type User struct {
	UserID      string    `json:"userId" bson:"user_id"`
	WorkspaceID string    `json:"workspaceId" bson:"workspace_id"`
	Username    string    `json:"username" bson:"username"`
	Email       string    `json:"email" bson:"email"`
	FullName    string    `json:"fullName" bson:"full_name"`
	IsAdmin     bool      `json:"isAdmin" bson:"is_admin"`
	PassHash    string    `json:"-" bson:"pass_hash"`
	CreatedAt   time.Time `json:"createdAt" bson:"created_at"`
}

type Workspace struct {
	WorkspaceID string    `json:"workspaceId" bson:"workspace_id"`
	Slug        string    `json:"slug" bson:"slug"`
	Name        string    `json:"name" bson:"name"`
	CreatedAt   time.Time `json:"createdAt" bson:"created_at"`
}

