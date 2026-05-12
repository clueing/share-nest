package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"file-service/internal/model"
	"file-service/internal/security"

	_ "modernc.org/sqlite"
)

type SQLiteRepo struct {
	db *sql.DB
}

func NewSQLite(path string) (*SQLiteRepo, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)
	return &SQLiteRepo{db: db}, nil
}

func (r *SQLiteRepo) Close() error {
	return r.db.Close()
}

func (r *SQLiteRepo) Init() error {
	schema := `
PRAGMA foreign_keys = ON;

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
	enabled INTEGER NOT NULL DEFAULT 1,
	created_at INTEGER NOT NULL,
	FOREIGN KEY(item_id) REFERENCES items(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_items_created_at ON items(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_shares_code ON shares(share_code);
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

	return r.backfillAccessTokens()
}

func (r *SQLiteRepo) CreateItemWithShare(ctx context.Context, item model.Item, passwordHash, passwordPlain string) (model.ItemSummary, error) {
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
		accessToken = security.RandomString(24)
	}
	for range 8 {
		shareCode = security.RandomString(10)
		_, err = tx.ExecContext(
			ctx,
			`INSERT INTO shares (item_id, share_code, password_hash, password_plain, access_token, enabled, created_at)
			 VALUES (?, ?, ?, ?, ?, 1, ?)`,
			itemID,
			shareCode,
			passwordHash,
			passwordPlain,
			accessToken,
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
	COALESCE(s.share_code, ''), COALESCE(s.access_token, ''), COALESCE(s.password_plain, ''), COALESCE(s.password_hash, ''), COALESCE(s.enabled, 0)
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
			&enabled,
		); err != nil {
			return nil, 0, err
		}

		row.CreatedAt = time.Unix(createdAt, 0)
		row.ShareEnabled = enabled == 1
		row.PasswordProtected = passwordHash != ""
		items = append(items, row)
	}

	return items, total, rows.Err()
}

func (r *SQLiteRepo) GetSharedItemByCode(ctx context.Context, code string) (model.SharedItem, error) {
	var item model.SharedItem
	var createdAt int64
	var enabled int

	err := r.db.QueryRowContext(ctx, `
SELECT
	i.id, i.kind, i.name, i.storage_path, i.content_text, i.mime_type, i.ext, i.size, i.sha256, i.created_at,
	s.share_code, s.access_token, s.password_plain, s.password_hash, s.enabled
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
		&enabled,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.SharedItem{}, sql.ErrNoRows
		}
		return model.SharedItem{}, err
	}

	item.CreatedAt = time.Unix(createdAt, 0)
	item.ShareEnabled = enabled == 1
	return item, nil
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
		if _, err := r.db.Exec(`UPDATE shares SET access_token = ? WHERE id = ?`, security.RandomString(24), id); err != nil {
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
