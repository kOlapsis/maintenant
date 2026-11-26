// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWarnMissingTrustedProxies(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"https base URL", Config{BaseURL: "https://now.example.com"}, true},
		{"https status URL", Config{BaseURL: "http://127.0.0.1:8080", StatusURL: "HTTPS://status.example.com"}, true},
		{"blank proxy list", Config{BaseURL: "https://now.example.com", TrustedProxies: " , "}, true},
		{"proxies listed", Config{BaseURL: "https://now.example.com", TrustedProxies: "172.18.0.1"}, false},
		{"plain http", Config{BaseURL: "http://192.0.2.10:8080"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			a := &App{cfg: tt.cfg, logger: slog.New(slog.NewTextHandler(&logs, nil))}
			a.warnMissingTrustedProxies()
			assert.Equal(t, tt.want, bytes.Contains(logs.Bytes(), []byte("level=WARN msg=\"MAINTENANT_TRUSTED_PROXIES is not set")), logs.String())
		})
	}
}
