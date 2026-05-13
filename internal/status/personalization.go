package status

import (
	"errors"
	"time"
)

type AssetRole string

const (
	AssetRoleLogo    AssetRole = "logo"
	AssetRoleFavicon AssetRole = "favicon"
	AssetRoleHero    AssetRole = "hero"
)

var (
	ErrAssetTooLarge        = errors.New("asset exceeds size cap")
	ErrAssetUnsupportedMIME = errors.New("asset MIME not allowed for this role")
	ErrInvalidScheme        = errors.New("URL scheme not in allowlist (http, https)")
	ErrInvalidHex           = errors.New("color must be #RRGGBB or #RRGGBBAA")
	ErrInvalidLocale        = errors.New("locale not supported")
	ErrInvalidDateFormat    = errors.New("date format must be 'relative' or 'absolute'")
	ErrInvalidTimezone      = errors.New("timezone must be a valid IANA identifier")
	ErrFieldTooLong         = errors.New("field exceeds maximum length")
	ErrNotFound             = errors.New("not found")
)

const (
	DefaultTitle                  = "System Status"
	DefaultSubtitle               = ""
	DefaultColorBg                = "#0B0E13"
	DefaultColorSurface           = "#12151C"
	DefaultColorBorder            = "#1F2937"
	DefaultColorText              = "#FFFFFF"
	DefaultColorAccent            = "#22C55E"
	DefaultColorStatusOperational = "#22C55E"
	DefaultColorStatusDegraded    = "#EAB308"
	DefaultColorStatusPartial     = "#F97316"
	DefaultColorStatusMajor       = "#EF4444"
	DefaultLocale                 = "en"
	DefaultDateFormat             = "relative"
)

type Settings struct {
	Version          int64
	Title            string
	Subtitle         string
	Colors           Palette
	Announcement     Announcement
	FooterTextMD     string
	FooterTextHTML   string
	Locale           string
	Timezone         string
	DateFormat       string
	UpdatedAt        time.Time
}

type Palette struct {
	Background          string
	Surface             string
	Border              string
	Text                string
	Accent              string
	StatusOperational   string
	StatusDegraded      string
	StatusPartialOutage string
	StatusMajorOutage   string
}

type Announcement struct {
	Enabled     bool
	MessageMD   string
	MessageHTML string
	URL         string
}

type Asset struct {
	Role      AssetRole
	MIME      string
	Bytes     []byte
	ByteSize  int
	AltText   string
	UpdatedAt time.Time
}

type FooterLink struct {
	ID        int64
	Position  int
	Label     string
	URL       string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type FAQItem struct {
	ID         int64
	Position   int
	Question   string
	AnswerMD   string
	AnswerHTML string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type ContrastWarning struct {
	Pair            string  `json:"pair"`
	Ratio           float64 `json:"ratio"`
	WCAGAAThreshold float64 `json:"wcag_aa_threshold"`
	Severity        string  `json:"severity"`
}

func DefaultSettings() Settings {
	return Settings{
		Version:  0,
		Title:    DefaultTitle,
		Subtitle: DefaultSubtitle,
		Colors: Palette{
			Background:          DefaultColorBg,
			Surface:             DefaultColorSurface,
			Border:              DefaultColorBorder,
			Text:                DefaultColorText,
			Accent:              DefaultColorAccent,
			StatusOperational:   DefaultColorStatusOperational,
			StatusDegraded:      DefaultColorStatusDegraded,
			StatusPartialOutage: DefaultColorStatusPartial,
			StatusMajorOutage:   DefaultColorStatusMajor,
		},
		Locale:     DefaultLocale,
		DateFormat: DefaultDateFormat,
	}
}
