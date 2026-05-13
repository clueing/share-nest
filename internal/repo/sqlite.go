package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"file-service/internal/model"
	"file-service/internal/security"

	_ "modernc.org/sqlite"
)

type SQLiteRepo struct {
	db                 *sql.DB
	accessLogRetention int
}

func NewSQLite(path string, accessLogRetention int) (*SQLiteRepo, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	maxConns := minInt(maxInt(runtime.NumCPU(), 2), 4)
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns)
	return &SQLiteRepo{db: db, accessLogRetention: accessLogRetention}, nil
}

func (r *SQLiteRepo) Close() error {
	return r.db.Close()
}

func (r *SQLiteRepo) Init() error {
	schema := `
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA busy_timeout = 5000;

CREATE TABLE IF NOT EXISTS items (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	kind TEXT NOT NULL,
	name TEXT NOT NULL,
	storage_path TEXT NOT NULL DEFAULT '',
	content_text TEXT NOT NULL DEFAULT '',
	mime_type TEXT NOT NULL DEFAULT '',
	ext TEXT NOT NULL DEFAULT '',
	size INTEGER NOT NULL DEFAULT 0,
	sha256 TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS shares (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	item_id INTEGER NOT NULL UNIQUE,
	share_code TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL DEFAULT '',
	password_plain TEXT NOT NULL DEFAULT '',
	access_token TEXT NOT NULL DEFAULT '',
	expires_at INTEGER NOT NULL DEFAULT 0,
	max_downloads INTEGER NOT NULL DEFAULT 0,
	download_count INTEGER NOT NULL DEFAULT 0,
	enabled INTEGER NOT NULL DEFAULT 1,
	created_at INTEGER NOT NULL,
	FOREIGN KEY(item_id) REFERENCES items(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS access_logs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	share_code TEXT NOT NULL DEFAULT '',
	item_name TEXT NOT NULL DEFAULT '',
	event_type TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT '',
	message TEXT NOT NULL DEFAULT '',
	client_ip TEXT NOT NULL DEFAULT '',
	user_agent TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_items_created_at ON items(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_shares_code ON shares(share_code);
CREATE INDEX IF NOT EXISTS idx_access_logs_created_at ON access_logs(created_at DESC);
`

	_, err := r.db.Exec(schema)
	if err != nil {
		return err
	}

	if err := r.ensureShareColumn(`ALTER TABLE shares ADD COLUMN access_token TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := r.ensureShareColumn(`ALTER TABLE shares ADD COLUMN password_plain TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := r.ensureShareColumn(`ALTER TABLE shares ADD COLUMN expires_at INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := r.ensureShareColumn(`ALTER TABLE shares ADD COLUMN max_downloads INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := r.ensureShareColumn(`ALTER TABLE shares ADD COLUMN download_count INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}

	return r.backfillAccessTokens()
}

func (r *SQLiteRepo) CreateItemWithShare(ctx context.Context, item model.Item, passwordHash, passwordPlain string, expiresAt *time.Time, maxDownloads int) (model.ItemSummary, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return model.ItemSummary{}, err
	}
	defer tx.Rollback()

	now := time.Now().Unix()
	res, err := tx.ExecContext(
		ctx,
		`INSERT INTO items (kind, name, storage_path, content_text, mime_type, ext, size, sha256, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.Kind,
		item.Name,
		item.StoragePath,
		item.ContentText,
		item.MIMEType,
		item.Ext,
		item.Size,
		item.SHA256,
		now,
	)
	if err != nil {
		return model.ItemSummary{}, err
	}

	itemID, err := res.LastInsertId()
	if err != nil {
		return model.ItemSummary{}, err
	}

	var shareCode string
	accessToken := ""
	if passwordHash != "" {
		accessToken, err = security.RandomString(24)
		if err != nil {
			return model.ItemSummary{}, err
		}
	}
	var expiresUnix int64
	if expiresAt != nil {
		expiresUnix = expiresAt.Unix()
	}
	for range 8 {
		shareCode, err = security.RandomString(10)
		if err != nil {
			return model.ItemSummary{}, err
		}
		_, err = tx.ExecContext(
			ctx,
			`INSERT INTO shares (item_id, share_code, password_hash, password_plain, access_token, expires_at, max_downloads, download_count, enabled, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 0, 1, ?)`,
			itemID,
			shareCode,
			passwordHash,
			passwordPlain,
			accessToken,
			expiresUnix,
			maxDownloads,
			now,
		)
		if err == nil {
			break
		}
	}
	if err != nil {
		return model.ItemSummary{}, fmt.Errorf("create share: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return model.ItemSummary{}, err
	}

	item.ID = itemID
	item.CreatedAt = time.Unix(now, 0)
	return model.ItemSummary{
		Item:              item,
		ShareCode:         shareCode,
		ShareAccessToken:  accessToken,
		SharePassword:     passwordPlain,
		ShareExpiresAt:    expiresAt,
		MaxDownloads:      maxDownloads,
		DownloadCount:     0,
		ShareEnabled:      true,
		PasswordProtected: passwordHash != "",
	}, nil
}

func (r *SQLiteRepo) ListItemsPage(ctx context.Context, offset, limit int) ([]model.ItemSummary, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM items`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT
	i.id, i.kind, i.name, i.storage_path, i.mime_type, i.ext, i.size, i.sha256, i.created_at,
	COALESCE(s.share_code, ''), COALESCE(s.access_token, ''), COALESCE(s.password_plain, ''), COALESCE(s.password_hash, ''),
	COALESCE(s.expires_at, 0), COALESCE(s.max_downloads, 0), COALESCE(s.download_count, 0), COALESCE(s.enabled, 0)
FROM items i
LEFT JOIN shares s ON s.item_id = i.id
ORDER BY i.created_at DESC
LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []model.ItemSummary
	for rows.Next() {
		var row model.ItemSummary
		var createdAt int64
		var expiresAt int64
		var passwordHash string
		var enabled int

		if err := rows.Scan(
			&row.ID,
			&row.Kind,
			&row.Name,
			&row.StoragePath,
			&row.MIMEType,
			&row.Ext,
			&row.Size,
			&row.SHA256,
			&createdAt,
			&row.ShareCode,
			&row.ShareAccessToken,
			&row.SharePassword,
			&passwordHash,
			&expiresAt,
			&row.MaxDownloads,
			&row.DownloadCount,
			&enabled,
		); err != nil {
			return nil, 0, err
		}

		row.CreatedAt = time.Unix(createdAt, 0)
		row.ShareExpiresAt = unixToTimePtr(expiresAt)
		row.ShareEnabled = enabled == 1
		row.PasswordProtected = passwordHash != ""
		items = append(items, row)
	}

	return items, total, rows.Err()
}

func (r *SQLiteRepo) GetSharedItemByCode(ctx context.Context, code string) (model.SharedItem, error) {
	var item model.SharedItem
	var createdAt int64
	var expiresAt int64
	var enabled int

	err := r.db.QueryRowContext(ctx, `
SELECT
	i.id, i.kind, i.name, i.storage_path, i.content_text, i.mime_type, i.ext, i.size, i.sha256, i.created_at,
	s.share_code, s.access_token, s.password_plain, s.password_hash, s.expires_at, s.max_downloads, s.download_count, s.enabled
FROM shares s
JOIN items i ON i.id = s.item_id
WHERE s.share_code = ?`,
		code,
	).Scan(
		&item.ID,
		&item.Kind,
		&item.Name,
		&item.StoragePath,
		&item.ContentText,
		&item.MIMEType,
		&item.Ext,
		&item.Size,
		&item.SHA256,
		&createdAt,
		&item.ShareCode,
		&item.AccessToken,
		&item.SharePassword,
		&item.PasswordHash,
		&expiresAt,
		&item.MaxDownloads,
		&item.DownloadCount,
		&enabled,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.SharedItem{}, sql.ErrNoRows
		}
		return model.SharedItem{}, err
	}

	item.CreatedAt = time.Unix(createdAt, 0)
	item.ShareExpiresAt = unixToTimePtr(expiresAt)
	item.ShareEnabled = enabled == 1
	return item, nil
}

func (r *SQLiteRepo) DeleteItems(ctx context.Context, itemIDs []int64) ([]model.Item, error) {
	if len(itemIDs) == 0 {
		return nil, nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	placeholders := repeatPlaceholders(len(itemIDs))
	args := make([]any, 0, len(itemIDs))
	for _, itemID := range itemIDs {
		args = append(args, itemID)
	}

	rows, err := tx.QueryContext(
		ctx,
		fmt.Sprintf(`SELECT id, kind, name, storage_path, content_text, mime_type, ext, size, sha256, created_at
		FROM items WHERE id IN (%s)`, placeholders),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.Item
	for rows.Next() {
		var item model.Item
		var createdAt int64
		if err := rows.Scan(
			&item.ID,
			&item.Kind,
			&item.Name,
			&item.StoragePath,
			&item.ContentText,
			&item.MIMEType,
			&item.Ext,
			&item.Size,
			&item.SHA256,
			&createdAt,
		); err != nil {
			return nil, err
		}
		item.CreatedAt = time.Unix(createdAt, 0)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM items WHERE id IN (%s)`, placeholders), args...); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *SQLiteRepo) ensureShareColumn(statement string) error {
	_, err := r.db.Exec(statement)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return err
	}
	return nil
}

func (r *SQLiteRepo) backfillAccessTokens() error {
	rows, err := r.db.Query(`SELECT id FROM shares WHERE password_hash <> '' AND access_token = ''`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range ids {
		token, err := security.RandomString(24)
		if err != nil {
			return err
		}
		if _, err := r.db.Exec(`UPDATE shares SET access_token = ? WHERE id = ?`, token, id); err != nil {
			return err
		}
	}
	return nil
}

func (r *SQLiteRepo) DeleteItem(ctx context.Context, itemID int64) (model.Item, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Item{}, err
	}
	defer tx.Rollback()

	var item model.Item
	var createdAt int64
	err = tx.QueryRowContext(
		ctx,
		`SELECT id, kind, name, storage_path, content_text, mime_type, ext, size, sha256, created_at
		 FROM items WHERE id = ?`,
		itemID,
	).Scan(
		&item.ID,
		&item.Kind,
		&item.Name,
		&item.StoragePath,
		&item.ContentText,
		&item.MIMEType,
		&item.Ext,
		&item.Size,
		&item.SHA256,
		&createdAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Item{}, sql.ErrNoRows
		}
		return model.Item{}, err
	}
	item.CreatedAt = time.Unix(createdAt, 0)

	if _, err := tx.ExecContext(ctx, `DELETE FROM items WHERE id = ?`, itemID); err != nil {
		return model.Item{}, err
	}

	if err := tx.Commit(); err != nil {
		return model.Item{}, err
	}
	return item, nil
}

func (r *SQLiteRepo) IncrementDownloadCount(ctx context.Context, itemID int64) (bool, error) {
	res, err := r.db.ExecContext(
		ctx,
		`UPDATE shares
		 SET download_count = download_count + 1
		 WHERE item_id = ? AND (max_downloads = 0 OR download_count < max_downloads)`,
		itemID,
	)
	if err != nil {
		return false, err
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

func (r *SQLiteRepo) CreateAccessLog(ctx context.Context, entry model.AccessLog) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO access_logs (share_code, item_name, event_type, status, message, client_ip, user_agent, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ShareCode,
		entry.ItemName,
		entry.EventType,
		entry.Status,
		entry.Message,
		entry.ClientIP,
		entry.UserAgent,
		time.Now().Unix(),
	)
	if err != nil {
		return err
	}
	if r.accessLogRetention > 0 {
		if _, err := r.db.ExecContext(ctx, `
DELETE FROM access_logs
WHERE id NOT IN (
	SELECT id
	FROM access_logs
	ORDER BY created_at DESC, id DESC
	LIMIT ?
)`, r.accessLogRetention); err != nil {
			return err
		}
	}
	return nil
}

func (r *SQLiteRepo) ListRecentAccessLogs(ctx context.Context, limit int) ([]model.AccessLog, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, share_code, item_name, event_type, status, message, client_ip, user_agent, created_at
		 FROM access_logs
		 ORDER BY created_at DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []model.AccessLog
	for rows.Next() {
		var logItem model.AccessLog
		var createdAt int64
		if err := rows.Scan(
			&logItem.ID,
			&logItem.ShareCode,
			&logItem.ItemName,
			&logItem.EventType,
			&logItem.Status,
			&logItem.Message,
			&logItem.ClientIP,
			&logItem.UserAgent,
			&createdAt,
		); err != nil {
			return nil, err
		}
		logItem.CreatedAt = time.Unix(createdAt, 0)
		logs = append(logs, logItem)
	}

	return logs, rows.Err()
}

func repeatPlaceholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", count), ",")
}

func unixToTimePtr(value int64) *time.Time {
	if value <= 0 {
		return nil
	}
	t := time.Unix(value, 0)
	return &t
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
