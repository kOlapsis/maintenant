package oauth

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestTruncateForLog(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"short value is kept", "maintenant-mcp", "maintenant-mcp"},
		{"value at the limit is kept", strings.Repeat("a", 200), strings.Repeat("a", 200)},
		{"long value is cut", strings.Repeat("a", 1<<20), strings.Repeat("a", 200)},
		{"a cut rune is dropped", strings.Repeat("a", 199) + "é", strings.Repeat("a", 199)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateForLog(tt.in)

			assert.Equal(t, tt.want, got)
			assert.True(t, utf8.ValidString(got))
		})
	}
}
