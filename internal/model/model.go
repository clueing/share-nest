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
	ShareExpiresAt    *time.Time
	MaxDownloads      int
	DownloadCount     int
	ShareEnabled      bool
	PasswordProtected bool
}

type SharedItem struct {
	Item
	ShareCode     string
	AccessToken   string
	SharePassword string
	ShareExpiresAt *time.Time
	MaxDownloads   int
	DownloadCount  int
	ShareEnabled   bool
	PasswordHash   string
}

type AccessLog struct {
	ID        int64
	ShareCode string
	ItemName  string
	EventType string
	Status    string
	Message   string
	ClientIP  string
	UserAgent string
	CreatedAt time.Time
}
