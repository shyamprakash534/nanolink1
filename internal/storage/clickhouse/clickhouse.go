package clickhouse
import (
        "context"
        "database/sql"

        _ "github.com/ClickHouse/clickhouse-go/v2"
        "github.com/nanolink/nanolink/internal/models"
)


type Client struct {
	db *sql.DB
}

func NewClickHouseClient(dsn string) (*Client, error) {
	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	return &Client{db: db}, nil
}

func (c *Client) InsertClickBatch(ctx context.Context, events []models.ClickEvent) error {
	if c.db == nil || len(events) == 0 {
		return nil
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO nanolink_analytics.clicks
		(short_code, clicked_at, ip_hash, country_code, city, device_type, browser, os, referrer)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, ev := range events {
		_, err := stmt.ExecContext(ctx,
			ev.ShortCode,
			ev.ClickedAt,
			ev.IPHash,
			ev.CountryCode,
			"Unknown",
			ev.DeviceType,
			ev.Browser,
			ev.OS,
			ev.Referrer,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (c *Client) GetStatsByShortCode(ctx context.Context, shortCode string) (*models.URLStatsResponse, error) {
	if c.db == nil {
		return &models.URLStatsResponse{
			ShortCode:       shortCode,
			ClicksByCountry: map[string]int64{},
			ClicksByDevice:  map[string]int64{},
			ClicksByReferrer: map[string]int64{},
			HourlyClicks:    []models.HourlyClickStat{},
		}, nil
	}

	// Temporary real query: total clicks.
	var total int64
	err := c.db.QueryRowContext(ctx, `
		SELECT count()
		FROM nanolink_analytics.clicks
		WHERE short_code = ?
	`, shortCode).Scan(&total)

	if err != nil {
		return nil, err
	}

	return &models.URLStatsResponse{
		ShortCode: shortCode,
		ClicksByCountry: map[string]int64{
			"total": total,
		},
		ClicksByDevice:   map[string]int64{},
		ClicksByReferrer: map[string]int64{},
		HourlyClicks:     []models.HourlyClickStat{},
	}, nil
}
