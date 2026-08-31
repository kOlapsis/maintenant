// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// TelegramAPIBase is the destination the product calls. The operator never
// supplies it: that is what makes the channel immune to the SSRF question a
// user-supplied URL raises.
const TelegramAPIBase = "https://api.telegram.org"

// A bot token is "<bot id>:<secret>". BotFather mints a 35-character secret;
// the length is not contractual, so the rule is a floor, not an equality.
var telegramTokenRe = regexp.MustCompile(`^[0-9]{5,}:[A-Za-z0-9_-]{30,}$`)

// A chat is either a numeric id — negative for groups and channels, often
// prefixed -100 — or a public @username.
var (
	telegramChatIDRe   = regexp.MustCompile(`^-?[0-9]{1,20}$`)
	telegramUsernameRe = regexp.MustCompile(`^@[A-Za-z][A-Za-z0-9_]{4,31}$`)
	telegramThreadIDRe = regexp.MustCompile(`^[0-9]{1,19}$`)
)

// TelegramConfig is the non-secret part of a Telegram channel, serialized into
// NotificationChannel.Config.
type TelegramConfig struct {
	ThreadID string `json:"thread_id,omitempty"`
}

// ParseTelegramConfig reads the channel's config column. An empty column is a
// channel with no topic, not an error.
func ParseTelegramConfig(raw string) (TelegramConfig, error) {
	var cfg TelegramConfig
	if strings.TrimSpace(raw) == "" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return cfg, fmt.Errorf("parse telegram config: %w", err)
	}
	return cfg, nil
}

// ValidateBotToken rejects a token whose shape cannot be a Telegram token. It
// says nothing about whether the token still works: that is what the test
// button is for. The point is to spare a network call, and a puzzling refusal
// from Telegram, on an obvious typo.
func ValidateBotToken(token string) error {
	if token == "" {
		return errors.New("bot token is required")
	}
	if !telegramTokenRe.MatchString(token) {
		return errors.New(`bot token must look like "123456789:AA...", as issued by @BotFather`)
	}
	return nil
}

// ValidateChatID accepts a numeric chat id, negative or not, or a public
// @username. The -100 prefix supergroups carry is part of the number and is
// passed through untouched.
func ValidateChatID(chatID string) error {
	if chatID == "" {
		return errors.New("chat id is required")
	}
	if telegramChatIDRe.MatchString(chatID) || telegramUsernameRe.MatchString(chatID) {
		return nil
	}
	return errors.New(`chat id must be a number (e.g. -1001234567890) or a public @username`)
}

// ValidateThreadID accepts an empty value: a channel without a topic posts to
// the group's general thread.
func ValidateThreadID(threadID string) error {
	if threadID == "" {
		return nil
	}
	if !telegramThreadIDRe.MatchString(threadID) {
		return errors.New("topic id must be a positive number")
	}
	return nil
}

// telegramMaxLen is the sendMessage limit. A longer message is not shortened by
// Telegram, it is refused, so truncating is on us (FR-012).
const telegramMaxLen = 4096

// telegramTruncationMark is what the reader sees where the text stops.
const telegramTruncationMark = "\n\n… (message truncated)"

// escapeTelegramHTML neutralizes the three characters Telegram's HTML parser
// treats as markup. Every dynamic value goes through it, so an alert whose text
// contains "a < b" is displayed, not rejected (FR-011).
func escapeTelegramHTML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

// BuildTelegramMessage renders the fired or resolved message. The severity and
// the entity sit on the first line so a phone notification, which shows little,
// still says what happened and to what (SC-006).
func BuildTelegramMessage(eventType string, a *Alert) string {
	resolved := strings.Contains(eventType, "resolved")

	emoji := severityEmoji(a.Severity)
	stamp := a.FiredAt
	stampLabel := "Fired"
	if resolved {
		emoji = "\xE2\x9C\x85" // white heavy check mark
		stampLabel = "Resolved"
		if a.ResolvedAt != nil {
			stamp = *a.ResolvedAt
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s <b>%s</b>\n\n", emoji, escapeTelegramHTML(eventTitle(eventType, a)))
	if !resolved {
		fmt.Fprintf(&b, "<b>Severity</b> · %s\n", escapeTelegramHTML(a.Severity))
	}
	fmt.Fprintf(&b, "<b>Source</b> · %s\n", escapeTelegramHTML(a.Source))
	fmt.Fprintf(&b, "<b>Entity</b> · %s\n", escapeTelegramHTML(a.EntityName))
	fmt.Fprintf(&b, "<b>%s</b> · %s\n", stampLabel, stamp.UTC().Format("2006-01-02 15:04:05 MST"))

	header := b.String()
	body := escapeTelegramHTML(a.Message)
	if body == "" {
		return header
	}

	// The header carries what must survive: severity, title, entity. Only the
	// description is cut.
	room := telegramMaxLen - len(header) - len("\n") - len(telegramTruncationMark)
	if len(body) > room {
		if room < 0 {
			room = 0
		}
		return header + "\n" + trimToValidUTF8(body[:room]) + telegramTruncationMark
	}
	return header + "\n" + body
}

// trimToValidUTF8 drops a trailing partial rune left by a byte-wise cut, and
// the escape sequence a cut may have split in half — "&am" would render as
// literal text where "&amp;" renders as "&".
func trimToValidUTF8(s string) string {
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	if i := strings.LastIndexByte(s, '&'); i >= 0 && !strings.ContainsAny(s[i:], ";") {
		s = s[:i]
	}
	return strings.TrimRight(s, " \n\t")
}

// TelegramRateLimitError carries the delay Telegram asked for. It is a typed
// error rather than a string the retry loop would have to read, so honouring
// the delay cannot break on a wording change (FR-014).
type TelegramRateLimitError struct {
	RetryAfter time.Duration
	Reason     string
}

func (e *TelegramRateLimitError) Error() string {
	return fmt.Sprintf("HTTP 429: %s (retry after %s)", e.Reason, e.RetryAfter)
}

// telegramResponse is the envelope every Bot API method answers with.
type telegramResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// SendTelegram posts one message.
//
// The client and the API base are parameters, not fields read from the caller:
// in production the notifier hands over its own client, SSRF guard included,
// and the real host; tests hand over a bare client and an httptest server,
// which the guard would otherwise refuse for listening on 127.0.0.1.
//
// The URL is never logged and never wrapped into an error: it carries the bot
// token (FR-005).
func SendTelegram(ctx context.Context, client *http.Client, apiBase string, ch *NotificationChannel, text string) error {
	cfg, err := ParseTelegramConfig(ch.Config)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"chat_id":                  ch.URL,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	}
	if cfg.ThreadID != "" {
		threadID, convErr := strconv.Atoi(cfg.ThreadID)
		if convErr != nil {
			return fmt.Errorf("invalid topic id %q", cfg.ThreadID)
		}
		payload["message_thread_id"] = threadID
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal telegram payload: %w", err)
	}

	endpoint := strings.TrimRight(apiBase, "/") + "/bot" + ch.Secret + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return errors.New("create telegram request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		// err carries the URL, and the URL carries the token.
		return errors.New("send to telegram: request failed")
	}
	defer func() { _ = resp.Body.Close() }()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var parsed telegramResponse
	_ = json.Unmarshal(raw, &parsed)

	if resp.StatusCode == http.StatusTooManyRequests {
		return &TelegramRateLimitError{
			RetryAfter: time.Duration(parsed.Parameters.RetryAfter) * time.Second,
			Reason:     telegramReason(parsed.Description),
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !parsed.OK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, telegramReasonFor(ch.URL, parsed.Description))
	}
	return nil
}

// telegramReason is what the operator reads: the phrase Telegram sent, which
// names the fix ("chat not found", "bot was kicked from the supergroup chat").
// A bare status code names nothing (FR-008).
func telegramReason(description string) string {
	if d := strings.TrimSpace(description); d != "" {
		return d
	}
	return "no reason given"
}

// telegramReasonFor adds what Telegram leaves out. "chat not found" on an
// @name is almost always the same mistake: an @name only resolves for a public
// channel, so a bot's own @name, or a private conversation, can never be one.
// Telegram answers the same three words either way, which sends the operator
// looking at the token instead of the destination.
func telegramReasonFor(chatID, description string) string {
	reason := telegramReason(description)
	if strings.HasPrefix(chatID, "@") && strings.Contains(strings.ToLower(reason), "chat not found") {
		return reason + " — an @name only works for a public channel; for a private conversation or a group, use the numeric chat id, and note that the bot's own @name is never a destination"
	}
	return reason
}
