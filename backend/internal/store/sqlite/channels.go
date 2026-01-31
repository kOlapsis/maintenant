package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kolapsis/pulseboard/internal/alert"
)

// ChannelStoreImpl implements alert.ChannelStore using SQLite.
type ChannelStoreImpl struct {
	db     *sql.DB
	writer *Writer
}

// NewChannelStore creates a new SQLite-backed channel store.
func NewChannelStore(d *DB) *ChannelStoreImpl {
	return &ChannelStoreImpl{
		db:     d.ReadDB(),
		writer: d.Writer(),
	}
}

func (s *ChannelStoreImpl) InsertChannel(ctx context.Context, ch *alert.NotificationChannel) (int64, error) {
	res, err := s.writer.Exec(ctx,
		`INSERT INTO notification_channels (name, type, url, headers, enabled)
		VALUES (?, ?, ?, ?, ?)`,
		ch.Name, ch.Type, ch.URL, NullableString(ch.Headers), boolToInt(ch.Enabled),
	)
	if err != nil {
		return 0, fmt.Errorf("insert channel: %w", err)
	}
	ch.ID = res.LastInsertID
	return res.LastInsertID, nil
}

func (s *ChannelStoreImpl) GetChannel(ctx context.Context, id int64) (*alert.NotificationChannel, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, type, url, headers, enabled, created_at, updated_at
		FROM notification_channels WHERE id = ?`, id)

	ch, err := scanChannel(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Load routing rules
	rules, err := s.ListRoutingRulesByChannel(ctx, ch.ID)
	if err != nil {
		return nil, err
	}
	ch.RoutingRules = rules

	return ch, nil
}

func (s *ChannelStoreImpl) ListChannels(ctx context.Context) ([]*alert.NotificationChannel, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, type, url, headers, enabled, created_at, updated_at
		FROM notification_channels ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	defer rows.Close()

	var channels []*alert.NotificationChannel
	for rows.Next() {
		ch, err := scanChannelRow(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load routing rules for each channel
	for _, ch := range channels {
		rules, err := s.ListRoutingRulesByChannel(ctx, ch.ID)
		if err != nil {
			return nil, err
		}
		ch.RoutingRules = rules
	}

	// Compute health for each channel
	for _, ch := range channels {
		health, err := s.GetChannelHealth(ctx, ch.ID)
		if err == nil {
			ch.Health = health
		}
	}

	return channels, nil
}

func (s *ChannelStoreImpl) UpdateChannel(ctx context.Context, ch *alert.NotificationChannel) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE notification_channels SET name=?, type=?, url=?, headers=?, enabled=?, updated_at=?
		WHERE id=?`,
		ch.Name, ch.Type, ch.URL, NullableString(ch.Headers), boolToInt(ch.Enabled),
		time.Now().UTC().Format(time.RFC3339), ch.ID,
	)
	if err != nil {
		return fmt.Errorf("update channel: %w", err)
	}
	return nil
}

func (s *ChannelStoreImpl) DeleteChannel(ctx context.Context, id int64) error {
	_, err := s.writer.Exec(ctx, `DELETE FROM notification_channels WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete channel: %w", err)
	}
	return nil
}

func (s *ChannelStoreImpl) GetChannelHealth(ctx context.Context, channelID int64) (string, error) {
	var status string
	err := s.db.QueryRowContext(ctx,
		`SELECT status FROM notification_deliveries
		WHERE channel_id = ? ORDER BY updated_at DESC LIMIT 1`,
		channelID).Scan(&status)
	if err == sql.ErrNoRows {
		return "healthy", nil
	}
	if err != nil {
		return "", err
	}
	if status == alert.DeliveryFailed {
		return "failing", nil
	}
	return "healthy", nil
}

// --- Routing Rules ---

func (s *ChannelStoreImpl) InsertRoutingRule(ctx context.Context, rule *alert.RoutingRule) (int64, error) {
	res, err := s.writer.Exec(ctx,
		`INSERT INTO routing_rules (channel_id, source_filter, severity_filter)
		VALUES (?, ?, ?)`,
		rule.ChannelID, NullableString(rule.SourceFilter), NullableString(rule.SeverityFilter),
	)
	if err != nil {
		return 0, fmt.Errorf("insert routing rule: %w", err)
	}
	rule.ID = res.LastInsertID
	return res.LastInsertID, nil
}

func (s *ChannelStoreImpl) DeleteRoutingRule(ctx context.Context, id int64) error {
	_, err := s.writer.Exec(ctx, `DELETE FROM routing_rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete routing rule: %w", err)
	}
	return nil
}

func (s *ChannelStoreImpl) ListRoutingRulesByChannel(ctx context.Context, channelID int64) ([]alert.RoutingRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, channel_id, source_filter, severity_filter, created_at
		FROM routing_rules WHERE channel_id = ?`, channelID)
	if err != nil {
		return nil, fmt.Errorf("list routing rules: %w", err)
	}
	defer rows.Close()

	var rules []alert.RoutingRule
	for rows.Next() {
		var r alert.RoutingRule
		var sourceFilter, severityFilter sql.NullString
		var createdAt string
		if err := rows.Scan(&r.ID, &r.ChannelID, &sourceFilter, &severityFilter, &createdAt); err != nil {
			return nil, err
		}
		if sourceFilter.Valid {
			r.SourceFilter = sourceFilter.String
		}
		if severityFilter.Valid {
			r.SeverityFilter = severityFilter.String
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// --- Notification Deliveries ---

func (s *ChannelStoreImpl) InsertDelivery(ctx context.Context, d *alert.NotificationDelivery) (int64, error) {
	res, err := s.writer.Exec(ctx,
		`INSERT INTO notification_deliveries (alert_id, channel_id, status, attempts)
		VALUES (?, ?, ?, ?)`,
		d.AlertID, d.ChannelID, d.Status, d.Attempts,
	)
	if err != nil {
		return 0, fmt.Errorf("insert delivery: %w", err)
	}
	d.ID = res.LastInsertID
	return res.LastInsertID, nil
}

func (s *ChannelStoreImpl) UpdateDelivery(ctx context.Context, d *alert.NotificationDelivery) error {
	_, err := s.writer.Exec(ctx,
		`UPDATE notification_deliveries SET status=?, attempts=?, last_error=?, updated_at=?
		WHERE id=?`,
		d.Status, d.Attempts, NullableString(d.LastError),
		time.Now().UTC().Format(time.RFC3339), d.ID,
	)
	if err != nil {
		return fmt.Errorf("update delivery: %w", err)
	}
	return nil
}

func (s *ChannelStoreImpl) ListDeliveriesByAlert(ctx context.Context, alertID int64) ([]*alert.NotificationDelivery, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, alert_id, channel_id, status, attempts, last_error, created_at, updated_at
		FROM notification_deliveries WHERE alert_id = ? ORDER BY created_at ASC`, alertID)
	if err != nil {
		return nil, fmt.Errorf("list deliveries: %w", err)
	}
	defer rows.Close()

	var deliveries []*alert.NotificationDelivery
	for rows.Next() {
		d := &alert.NotificationDelivery{}
		var lastError sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&d.ID, &d.AlertID, &d.ChannelID, &d.Status, &d.Attempts, &lastError, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if lastError.Valid {
			d.LastError = lastError.String
		}
		d.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		d.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}

// --- Scan helpers ---

func scanChannel(row *sql.Row) (*alert.NotificationChannel, error) {
	ch := &alert.NotificationChannel{}
	var headers sql.NullString
	var enabled int
	var createdAt, updatedAt string

	err := row.Scan(&ch.ID, &ch.Name, &ch.Type, &ch.URL, &headers, &enabled, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	ch.Enabled = enabled == 1
	if headers.Valid {
		ch.Headers = headers.String
	}
	ch.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	ch.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return ch, nil
}

func scanChannelRow(rows *sql.Rows) (*alert.NotificationChannel, error) {
	ch := &alert.NotificationChannel{}
	var headers sql.NullString
	var enabled int
	var createdAt, updatedAt string

	err := rows.Scan(&ch.ID, &ch.Name, &ch.Type, &ch.URL, &headers, &enabled, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	ch.Enabled = enabled == 1
	if headers.Valid {
		ch.Headers = headers.String
	}
	ch.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	ch.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return ch, nil
}
