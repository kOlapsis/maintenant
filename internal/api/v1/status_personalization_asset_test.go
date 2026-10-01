// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func putLogo(t *testing.T, h *PersonalizationHandler, content string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "logo.svg")
	require.NoError(t, err)
	_, err = part.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	req := httptest.NewRequest(http.MethodPut, "/api/v1/status-page/assets/logo", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.SetPathValue("role", "logo")
	rec := httptest.NewRecorder()
	h.HandlePutAsset(rec, req)
	return rec
}

func TestPutAsset_SVGIsReadWhole(t *testing.T) {
	h := newTestPersoHandler(t)
	padding := `<rect width="10" height="10" fill="#000"/>` + strings.Repeat(" ", 600)

	rec := putLogo(t, h, `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg">`+padding+`<script>alert(1)</script></svg>`)
	assert.Equal(t, http.StatusBadRequest, rec.Code, "a script past the first 512 bytes is still a script: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), "active_svg")

	rec = putLogo(t, h, `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg">`+padding+`</svg>`)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}
