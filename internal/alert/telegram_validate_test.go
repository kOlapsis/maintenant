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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FR-004: a malformed token is refused on shape alone, with no call to
// Telegram. These functions take no context and no client, which is what makes
// that structural rather than a promise.
func TestValidateBotToken(t *testing.T) {
	valid := []string{
		"8123456789:AAFxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		"123456:AAH-_zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
	}
	for _, token := range valid {
		assert.NoError(t, ValidateBotToken(token), "token %q", token)
	}

	invalid := map[string]string{
		"empty":            "",
		"no colon":         "8123456789AAFxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		"no bot id":        ":AAFxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		"bot id not digit": "abc123:AAFxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		"secret too short": "8123456789:AAFshort",
		"spaces":           "8123456789:AAF xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		"a whole url":      "https://api.telegram.org/bot8123456789:AAFxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
	}
	for name, token := range invalid {
		assert.Error(t, ValidateBotToken(token), "case %q should be refused", name)
	}
}

// A group id is negative and often carries the -100 prefix. Refusing it as
// "invalid" is the mistake this test exists to prevent.
func TestValidateChatID(t *testing.T) {
	valid := []string{"-1001234567890", "-100123", "-123456", "123456", "@maintenant_alerts"}
	for _, id := range valid {
		assert.NoError(t, ValidateChatID(id), "chat id %q", id)
	}

	invalid := map[string]string{
		"empty":              "",
		"not a number":       "my-group",
		"bare at":            "@",
		"username too short": "@abc",
		"url":                "https://t.me/mygroup",
		"two numbers":        "-100123 456",
	}
	for name, id := range invalid {
		assert.Error(t, ValidateChatID(id), "case %q should be refused", name)
	}
}

func TestValidateThreadID(t *testing.T) {
	assert.NoError(t, ValidateThreadID(""), "no topic is the general thread, not an error")
	assert.NoError(t, ValidateThreadID("42"))

	for _, id := range []string{"-1", "4.2", "abc", "42 "} {
		assert.Error(t, ValidateThreadID(id), "thread id %q", id)
	}
}

func TestParseTelegramConfig(t *testing.T) {
	cfg, err := ParseTelegramConfig("")
	require.NoError(t, err)
	assert.Empty(t, cfg.ThreadID)

	cfg, err = ParseTelegramConfig(`{"thread_id":"42"}`)
	require.NoError(t, err)
	assert.Equal(t, "42", cfg.ThreadID)

	_, err = ParseTelegramConfig("{not json")
	assert.Error(t, err)
}
