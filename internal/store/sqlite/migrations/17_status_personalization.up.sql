CREATE TABLE status_page_settings (
  id                       INTEGER PRIMARY KEY CHECK (id = 1),
  version                  INTEGER NOT NULL DEFAULT 0,

  title                    TEXT NOT NULL DEFAULT 'System Status',
  subtitle                 TEXT NOT NULL DEFAULT '',

  color_bg                 TEXT NOT NULL DEFAULT '#0B0E13',
  color_surface            TEXT NOT NULL DEFAULT '#12151C',
  color_border             TEXT NOT NULL DEFAULT '#1F2937',
  color_text               TEXT NOT NULL DEFAULT '#FFFFFF',
  color_accent             TEXT NOT NULL DEFAULT '#22C55E',

  color_status_operational TEXT NOT NULL DEFAULT '#22C55E',
  color_status_degraded    TEXT NOT NULL DEFAULT '#EAB308',
  color_status_partial     TEXT NOT NULL DEFAULT '#F97316',
  color_status_major       TEXT NOT NULL DEFAULT '#EF4444',

  announcement_enabled       INTEGER NOT NULL DEFAULT 0,
  announcement_message_md    TEXT NOT NULL DEFAULT '',
  announcement_message_html  TEXT NOT NULL DEFAULT '',
  announcement_url           TEXT NOT NULL DEFAULT '',

  footer_text_md             TEXT NOT NULL DEFAULT '',
  footer_text_html           TEXT NOT NULL DEFAULT '',

  locale                     TEXT NOT NULL DEFAULT 'en',
  timezone                   TEXT NOT NULL DEFAULT '',
  date_format                TEXT NOT NULL DEFAULT 'relative',

  updated_at                 INTEGER NOT NULL DEFAULT 0
);

INSERT INTO status_page_settings (id) VALUES (1);

CREATE TABLE status_page_assets (
  role        TEXT PRIMARY KEY CHECK (role IN ('logo', 'favicon', 'hero')),
  mime        TEXT NOT NULL,
  bytes       BLOB NOT NULL,
  byte_size   INTEGER NOT NULL,
  alt_text    TEXT NOT NULL DEFAULT '',
  updated_at  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE status_page_footer_links (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  position    INTEGER NOT NULL DEFAULT 0,
  label       TEXT NOT NULL DEFAULT '',
  url         TEXT NOT NULL,
  created_at  INTEGER NOT NULL DEFAULT 0,
  updated_at  INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_status_page_footer_links_position ON status_page_footer_links(position);

CREATE TABLE status_page_faq_items (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  position    INTEGER NOT NULL DEFAULT 0,
  question    TEXT NOT NULL,
  answer_md   TEXT NOT NULL,
  answer_html TEXT NOT NULL,
  created_at  INTEGER NOT NULL DEFAULT 0,
  updated_at  INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_status_page_faq_items_position ON status_page_faq_items(position);
