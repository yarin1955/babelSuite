package domain

import "time"

type FavoritePackage struct {
	UserID    string    `json:"userId" bson:"user_id"`
	PackageID string    `json:"packageId" bson:"package_id"`
	CreatedAt time.Time `json:"createdAt" bson:"created_at"`
}
