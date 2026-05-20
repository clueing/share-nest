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
	ShareCode      string
	AccessToken    string
	SharePassword  string
	ShareExpiresAt *time.Time
	MaxDownloads   int
	DownloadCount  int
	ShareEnabled   bool
	PasswordHash   string
}

type AccessLog struct {
	ID        int64
	ItemID    int64
	ShareCode string
	ItemName  string
	EventType string
	Status    string
	Message   string
	ClientIP  string
	UserAgent string
	CreatedAt time.Time
}

type ItemQuery struct {
	Keyword string
	Kind    string
	Status  string
	Offset  int
	Limit   int
}

type ShareQuery struct {
	Keyword string
	Status  string
	Offset  int
	Limit   int
}

type DashboardStats struct {
	TotalItems     int
	FileItems      int
	TextItems      int
	TotalShares    int
	ActiveShares   int
	ExpiredShares  int
	TotalDownloads int
	TodayDownloads int
}

type SystemSetting struct {
	Key       string
	Value     string
	UpdatedAt time.Time
}
