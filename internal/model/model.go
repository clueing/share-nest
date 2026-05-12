package model

import "time"

type Item struct {
	ID          int64
	Kind        string
	Name        string
	StoragePath string
	ContentText string
	MIMEType    string
	Ext         string
	Size        int64
	SHA256      string
	CreatedAt   time.Time
}

type ItemSummary struct {
	Item
	ShareCode         string
	ShareAccessToken  string
	SharePassword     string
	ShareEnabled      bool
	PasswordProtected bool
}

type SharedItem struct {
	Item
	ShareCode    string
	AccessToken  string
	SharePassword string
	ShareEnabled bool
	PasswordHash string
}
