package postgres

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/nanolink/nanolink/internal/models"
)

type URLRepository struct {
	db *DB
}

func NewURLRepository(db *DB) *URLRepository {
	return &URLRepository{db: db}
}

func (r *URLRepository) CreateURL(url *models.URL) error {
	query := `
		INSERT INTO urls (short_code, long_url, custom_alias, user_id, created_at, expires_at, click_count, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`
	var id uint64
	err := r.db.QueryRow(
		query,
		url.ShortCode,
		url.LongURL,
		url.CustomAlias,
		url.UserID,
		url.CreatedAt,
		url.ExpiresAt,
		url.ClickCount,
		url.IsActive,
	).Scan(&id)

	if err != nil {
		return err
	}
	url.ID = id
	return nil
}

func (r *URLRepository) GetByShortCode(shortCode string) (*models.URL, error) {
	query := `
		SELECT id, short_code, long_url, custom_alias, user_id, created_at, expires_at, click_count, is_active
		FROM urls
		WHERE short_code = $1 AND is_active = TRUE
	`
	row := r.db.QueryRow(query, shortCode)
	var url models.URL
	var userID sql.NullString

	err := row.Scan(
		&url.ID,
		&url.ShortCode,
		&url.LongURL,
		&url.CustomAlias,
		&userID,
		&url.CreatedAt,
		&url.ExpiresAt,
		&url.ClickCount,
		&url.IsActive,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if userID.Valid {
		uid, err := uuid.Parse(userID.String)
		if err == nil {
			url.UserID = &uid
		}
	}

	return &url, nil
}

func (r *URLRepository) IncrementClickCount(shortCode string) error {
	query := `
		UPDATE urls
		SET click_count = click_count + 1
		WHERE short_code = $1
	`
	_, err := r.db.Exec(query, shortCode)
	return err
}

func (r *URLRepository) DeleteURL(shortCode string, userID *uuid.UUID) error {
	var query string
	var err error
	if userID != nil {
		query = `UPDATE urls SET is_active = FALSE WHERE short_code = $1 AND user_id = $2`
		_, err = r.db.Exec(query, shortCode, userID)
	} else {
		query = `UPDATE urls SET is_active = FALSE WHERE short_code = $1`
		_, err = r.db.Exec(query, shortCode)
	}
	return err
}

func (r *URLRepository) ListURLsByUser(userID uuid.UUID, limit, offset int) ([]models.URL, error) {
	query := `
		SELECT id, short_code, long_url, custom_alias, user_id, created_at, expires_at, click_count, is_active
		FROM urls
		WHERE user_id = $1 AND is_active = TRUE
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.Query(query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var urls []models.URL
	for rows.Next() {
		var url models.URL
		var uid sql.NullString
		err := rows.Scan(
			&url.ID,
			&url.ShortCode,
			&url.LongURL,
			&url.CustomAlias,
			&uid,
			&url.CreatedAt,
			&url.ExpiresAt,
			&url.ClickCount,
			&url.IsActive,
		)
		if err != nil {
			return nil, err
		}
		if uid.Valid {
			u, _ := uuid.Parse(uid.String)
			url.UserID = &u
		}
		urls = append(urls, url)
	}
	return urls, nil
}
