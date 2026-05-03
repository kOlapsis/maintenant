package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kolapsis/maintenant/internal/status"
)

type PersonalizationStoreImpl struct {
	db     *sql.DB
	writer *Writer
}

func NewPersonalizationStore(d *DB) *PersonalizationStoreImpl {
	return &PersonalizationStoreImpl{
		db:     d.ReadDB(),
		writer: d.Writer(),
	}
}

func (s *PersonalizationStoreImpl) GetSettings(ctx context.Context) (status.Settings, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT version, title, subtitle,
			color_bg, color_surface, color_border, color_text, color_accent,
			color_status_operational, color_status_degraded, color_status_partial, color_status_major,
			announcement_enabled, announcement_message_md, announcement_message_html, announcement_url,
			footer_text_md, footer_text_html,
			locale, timezone, date_format, updated_at
		FROM status_page_settings WHERE id = 1`)

	var s2 status.Settings
	var updatedAt int64
	var announcementEnabled int
	err := row.Scan(
		&s2.Version, &s2.Title, &s2.Subtitle,
		&s2.Colors.Background, &s2.Colors.Surface, &s2.Colors.Border, &s2.Colors.Text, &s2.Colors.Accent,
		&s2.Colors.StatusOperational, &s2.Colors.StatusDegraded, &s2.Colors.StatusPartialOutage, &s2.Colors.StatusMajorOutage,
		&announcementEnabled, &s2.Announcement.MessageMD, &s2.Announcement.MessageHTML, &s2.Announcement.URL,
		&s2.FooterTextMD, &s2.FooterTextHTML,
		&s2.Locale, &s2.Timezone, &s2.DateFormat, &updatedAt,
	)
	if err != nil {
		return status.Settings{}, fmt.Errorf("get settings: %w", err)
	}
	s2.Announcement.Enabled = announcementEnabled != 0
	s2.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return s2, nil
}

func (s *PersonalizationStoreImpl) UpdateSettings(ctx context.Context, in status.Settings) (status.Settings, error) {
	now := time.Now().Unix()
	announcementEnabled := 0
	if in.Announcement.Enabled {
		announcementEnabled = 1
	}

	_, err := s.writer.Exec(ctx, `
		UPDATE status_page_settings SET
			version = version + 1,
			title = ?, subtitle = ?,
			color_bg = ?, color_surface = ?, color_border = ?, color_text = ?, color_accent = ?,
			color_status_operational = ?, color_status_degraded = ?, color_status_partial = ?, color_status_major = ?,
			announcement_enabled = ?, announcement_message_md = ?, announcement_message_html = ?, announcement_url = ?,
			footer_text_md = ?, footer_text_html = ?,
			locale = ?, timezone = ?, date_format = ?,
			updated_at = ?
		WHERE id = 1`,
		in.Title, in.Subtitle,
		in.Colors.Background, in.Colors.Surface, in.Colors.Border, in.Colors.Text, in.Colors.Accent,
		in.Colors.StatusOperational, in.Colors.StatusDegraded, in.Colors.StatusPartialOutage, in.Colors.StatusMajorOutage,
		announcementEnabled, in.Announcement.MessageMD, in.Announcement.MessageHTML, in.Announcement.URL,
		in.FooterTextMD, in.FooterTextHTML,
		in.Locale, in.Timezone, in.DateFormat,
		now,
	)
	if err != nil {
		return status.Settings{}, fmt.Errorf("update settings: %w", err)
	}
	return s.GetSettings(ctx)
}

func (s *PersonalizationStoreImpl) GetAsset(ctx context.Context, role status.AssetRole) (*status.Asset, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT role, mime, bytes, byte_size, alt_text, updated_at FROM status_page_assets WHERE role = ?`, string(role))

	var a status.Asset
	var updatedAt int64
	var roleStr string
	err := row.Scan(&roleStr, &a.MIME, &a.Bytes, &a.ByteSize, &a.AltText, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get asset %s: %w", role, err)
	}
	a.Role = status.AssetRole(roleStr)
	a.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return &a, nil
}

func (s *PersonalizationStoreImpl) PutAsset(ctx context.Context, a status.Asset) error {
	now := time.Now().Unix()
	// INSERT OR REPLACE is atomic in SQLite
	_, err := s.writer.Exec(ctx,
		`INSERT OR REPLACE INTO status_page_assets (role, mime, bytes, byte_size, alt_text, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		string(a.Role), a.MIME, a.Bytes, len(a.Bytes), a.AltText, now)
	if err != nil {
		return fmt.Errorf("put asset %s: %w", a.Role, err)
	}
	return nil
}

func (s *PersonalizationStoreImpl) DeleteAsset(ctx context.Context, role status.AssetRole) error {
	_, err := s.writer.Exec(ctx, `DELETE FROM status_page_assets WHERE role = ?`, string(role))
	return err
}

func (s *PersonalizationStoreImpl) ListFooterLinks(ctx context.Context) ([]status.FooterLink, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, position, label, url, created_at, updated_at FROM status_page_footer_links ORDER BY position, id`)
	if err != nil {
		return nil, fmt.Errorf("list footer links: %w", err)
	}
	defer rows.Close()
	return scanFooterLinks(rows)
}

func (s *PersonalizationStoreImpl) CreateFooterLink(ctx context.Context, label, url string) (status.FooterLink, error) {
	now := time.Now().Unix()
	var maxPos sql.NullInt64
	_ = s.db.QueryRowContext(ctx, `SELECT MAX(position) FROM status_page_footer_links`).Scan(&maxPos)
	pos := 0
	if maxPos.Valid {
		pos = int(maxPos.Int64) + 1
	}

	res, err := s.writer.Exec(ctx,
		`INSERT INTO status_page_footer_links (position, label, url, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		pos, label, url, now, now)
	if err != nil {
		return status.FooterLink{}, fmt.Errorf("create footer link: %w", err)
	}
	return status.FooterLink{
		ID:        res.LastInsertID,
		Position:  pos,
		Label:     label,
		URL:       url,
		CreatedAt: time.Unix(now, 0).UTC(),
		UpdatedAt: time.Unix(now, 0).UTC(),
	}, nil
}

func (s *PersonalizationStoreImpl) UpdateFooterLink(ctx context.Context, id int64, label, url string) (status.FooterLink, error) {
	now := time.Now().Unix()
	res, err := s.writer.Exec(ctx,
		`UPDATE status_page_footer_links SET label = ?, url = ?, updated_at = ? WHERE id = ?`,
		label, url, now, id)
	if err != nil {
		return status.FooterLink{}, fmt.Errorf("update footer link: %w", err)
	}
	if res.RowsAffected == 0 {
		return status.FooterLink{}, status.ErrNotFound
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, position, label, url, created_at, updated_at FROM status_page_footer_links WHERE id = ?`, id)
	return scanFooterLink(row)
}

func (s *PersonalizationStoreImpl) DeleteFooterLink(ctx context.Context, id int64) error {
	res, err := s.writer.Exec(ctx, `DELETE FROM status_page_footer_links WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return status.ErrNotFound
	}
	return nil
}

func (s *PersonalizationStoreImpl) ReorderFooterLinks(ctx context.Context, ids []int64) ([]status.FooterLink, error) {
	for pos, id := range ids {
		res, err := s.writer.Exec(ctx,
			`UPDATE status_page_footer_links SET position = ? WHERE id = ?`, pos, id)
		if err != nil {
			return nil, fmt.Errorf("reorder footer link %d: %w", id, err)
		}
		if res.RowsAffected == 0 {
			return nil, fmt.Errorf("footer link %d not found", id)
		}
	}
	return s.ListFooterLinks(ctx)
}

func (s *PersonalizationStoreImpl) ListFAQItems(ctx context.Context) ([]status.FAQItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, position, question, answer_md, answer_html, created_at, updated_at FROM status_page_faq_items ORDER BY position, id`)
	if err != nil {
		return nil, fmt.Errorf("list faq items: %w", err)
	}
	defer rows.Close()
	return scanFAQItems(rows)
}

func (s *PersonalizationStoreImpl) CreateFAQItem(ctx context.Context, question, answerMD, answerHTML string) (status.FAQItem, error) {
	now := time.Now().Unix()
	var maxPos sql.NullInt64
	_ = s.db.QueryRowContext(ctx, `SELECT MAX(position) FROM status_page_faq_items`).Scan(&maxPos)
	pos := 0
	if maxPos.Valid {
		pos = int(maxPos.Int64) + 1
	}

	res, err := s.writer.Exec(ctx,
		`INSERT INTO status_page_faq_items (position, question, answer_md, answer_html, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		pos, question, answerMD, answerHTML, now, now)
	if err != nil {
		return status.FAQItem{}, fmt.Errorf("create faq item: %w", err)
	}
	return status.FAQItem{
		ID:         res.LastInsertID,
		Position:   pos,
		Question:   question,
		AnswerMD:   answerMD,
		AnswerHTML: answerHTML,
		CreatedAt:  time.Unix(now, 0).UTC(),
		UpdatedAt:  time.Unix(now, 0).UTC(),
	}, nil
}

func (s *PersonalizationStoreImpl) UpdateFAQItem(ctx context.Context, id int64, question, answerMD, answerHTML string) (status.FAQItem, error) {
	now := time.Now().Unix()
	res, err := s.writer.Exec(ctx,
		`UPDATE status_page_faq_items SET question = ?, answer_md = ?, answer_html = ?, updated_at = ? WHERE id = ?`,
		question, answerMD, answerHTML, now, id)
	if err != nil {
		return status.FAQItem{}, fmt.Errorf("update faq item: %w", err)
	}
	if res.RowsAffected == 0 {
		return status.FAQItem{}, status.ErrNotFound
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, position, question, answer_md, answer_html, created_at, updated_at FROM status_page_faq_items WHERE id = ?`, id)
	return scanFAQItem(row)
}

func (s *PersonalizationStoreImpl) DeleteFAQItem(ctx context.Context, id int64) error {
	res, err := s.writer.Exec(ctx, `DELETE FROM status_page_faq_items WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return status.ErrNotFound
	}
	return nil
}

func (s *PersonalizationStoreImpl) ReorderFAQItems(ctx context.Context, ids []int64) ([]status.FAQItem, error) {
	for pos, id := range ids {
		res, err := s.writer.Exec(ctx,
			`UPDATE status_page_faq_items SET position = ? WHERE id = ?`, pos, id)
		if err != nil {
			return nil, fmt.Errorf("reorder faq item %d: %w", id, err)
		}
		if res.RowsAffected == 0 {
			return nil, fmt.Errorf("faq item %d not found", id)
		}
	}
	return s.ListFAQItems(ctx)
}

func scanFooterLinks(rows *sql.Rows) ([]status.FooterLink, error) {
	var links []status.FooterLink
	for rows.Next() {
		var link status.FooterLink
		var createdAt, updatedAt int64
		if err := rows.Scan(&link.ID, &link.Position, &link.Label, &link.URL, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan footer link: %w", err)
		}
		link.CreatedAt = time.Unix(createdAt, 0).UTC()
		link.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		links = append(links, link)
	}
	return links, rows.Err()
}

func scanFooterLink(row *sql.Row) (status.FooterLink, error) {
	var link status.FooterLink
	var createdAt, updatedAt int64
	if err := row.Scan(&link.ID, &link.Position, &link.Label, &link.URL, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return status.FooterLink{}, status.ErrNotFound
		}
		return status.FooterLink{}, fmt.Errorf("scan footer link: %w", err)
	}
	link.CreatedAt = time.Unix(createdAt, 0).UTC()
	link.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return link, nil
}

func scanFAQItems(rows *sql.Rows) ([]status.FAQItem, error) {
	var items []status.FAQItem
	for rows.Next() {
		var item status.FAQItem
		var createdAt, updatedAt int64
		if err := rows.Scan(&item.ID, &item.Position, &item.Question, &item.AnswerMD, &item.AnswerHTML, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan faq item: %w", err)
		}
		item.CreatedAt = time.Unix(createdAt, 0).UTC()
		item.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanFAQItem(row *sql.Row) (status.FAQItem, error) {
	var item status.FAQItem
	var createdAt, updatedAt int64
	if err := row.Scan(&item.ID, &item.Position, &item.Question, &item.AnswerMD, &item.AnswerHTML, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return status.FAQItem{}, status.ErrNotFound
		}
		return status.FAQItem{}, fmt.Errorf("scan faq item: %w", err)
	}
	item.CreatedAt = time.Unix(createdAt, 0).UTC()
	item.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return item, nil
}
