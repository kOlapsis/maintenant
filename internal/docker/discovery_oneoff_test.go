package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsOneOff(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   bool
	}{
		{
			// This is what `docker compose run` actually writes: capital T.
			name:   "compose run container",
			labels: map[string]string{"com.docker.compose.oneoff": "True"},
			want:   true,
		},
		{
			name:   "lowercase spelling",
			labels: map[string]string{"com.docker.compose.oneoff": "true"},
			want:   true,
		},
		{
			// `docker compose up` stamps the same key with False, so the value
			// is what decides, not the presence of the label.
			name:   "compose up container",
			labels: map[string]string{"com.docker.compose.oneoff": "False"},
			want:   false,
		},
		{
			name:   "service container carrying our labels",
			labels: map[string]string{"maintenant.endpoint.http": "https://example.com/health"},
			want:   false,
		},
		{
			name:   "no labels at all",
			labels: nil,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsOneOff(tt.labels))
		})
	}
}
