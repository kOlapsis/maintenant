package v1

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- In-memory mock store ---

type mockPersoStore struct {
	settings    status.Settings
	assets      map[status.AssetRole]*status.Asset
	footerLinks []status.FooterLink
	faqItems    []status.FAQItem
	nextID      int64
}

func newMockPersoStore() *mockPersoStore {
	return &mockPersoStore{
		settings: status.DefaultSettings(),
		assets:   make(map[status.AssetRole]*status.Asset),
	}
}

func (m *mockPersoStore) GetSettings(_ context.Context) (status.Settings, error) {
	return m.settings, nil
}

func (m *mockPersoStore) UpdateSettings(_ context.Context, s status.Settings) (status.Settings, error) {
	s.Version = m.settings.Version + 1
	s.UpdatedAt = time.Now().UTC()
	m.settings = s
	return m.settings, nil
}

func (m *mockPersoStore) BumpVersion(_ context.Context) error {
	m.settings.Version++
	m.settings.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *mockPersoStore) GetAsset(_ context.Context, role status.AssetRole) (*status.Asset, error) {
	return m.assets[role], nil
}

func (m *mockPersoStore) PutAsset(_ context.Context, a status.Asset) error {
	m.assets[a.Role] = &a
	return nil
}

func (m *mockPersoStore) DeleteAsset(_ context.Context, role status.AssetRole) error {
	delete(m.assets, role)
	return nil
}

func (m *mockPersoStore) ListFooterLinks(_ context.Context) ([]status.FooterLink, error) {
	return m.footerLinks, nil
}

func (m *mockPersoStore) CreateFooterLink(_ context.Context, label, url string) (status.FooterLink, error) {
	m.nextID++
	l := status.FooterLink{ID: m.nextID, Label: label, URL: url, Position: len(m.footerLinks) + 1}
	m.footerLinks = append(m.footerLinks, l)
	return l, nil
}

func (m *mockPersoStore) UpdateFooterLink(_ context.Context, id int64, label, url string) (status.FooterLink, error) {
	for i, l := range m.footerLinks {
		if l.ID == id {
			m.footerLinks[i].Label = label
			m.footerLinks[i].URL = url
			return m.footerLinks[i], nil
		}
	}
	return status.FooterLink{}, status.ErrNotFound
}

func (m *mockPersoStore) DeleteFooterLink(_ context.Context, id int64) error {
	for i, l := range m.footerLinks {
		if l.ID == id {
			m.footerLinks = append(m.footerLinks[:i], m.footerLinks[i+1:]...)
			return nil
		}
	}
	return status.ErrNotFound
}

func (m *mockPersoStore) ReorderFooterLinks(_ context.Context, _ []int64) ([]status.FooterLink, error) {
	return m.footerLinks, nil
}

func (m *mockPersoStore) ListFAQItems(_ context.Context) ([]status.FAQItem, error) {
	return m.faqItems, nil
}

func (m *mockPersoStore) CreateFAQItem(_ context.Context, question, answerMD, answerHTML string) (status.FAQItem, error) {
	m.nextID++
	item := status.FAQItem{ID: m.nextID, Question: question, AnswerMD: answerMD, AnswerHTML: answerHTML, Position: len(m.faqItems) + 1}
	m.faqItems = append(m.faqItems, item)
	return item, nil
}

func (m *mockPersoStore) UpdateFAQItem(_ context.Context, id int64, question, answerMD, answerHTML string) (status.FAQItem, error) {
	for i, item := range m.faqItems {
		if item.ID == id {
			m.faqItems[i].Question = question
			m.faqItems[i].AnswerMD = answerMD
			m.faqItems[i].AnswerHTML = answerHTML
			return m.faqItems[i], nil
		}
	}
	return status.FAQItem{}, status.ErrNotFound
}

func (m *mockPersoStore) DeleteFAQItem(_ context.Context, id int64) error {
	for i, item := range m.faqItems {
		if item.ID == id {
			m.faqItems = append(m.faqItems[:i], m.faqItems[i+1:]...)
			return nil
		}
	}
	return status.ErrNotFound
}

func (m *mockPersoStore) ReorderFAQItems(_ context.Context, _ []int64) ([]status.FAQItem, error) {
	return m.faqItems, nil
}

func newTestPersoHandler(t *testing.T) *PersonalizationHandler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := status.NewPersonalizationService(newMockPersoStore(), logger)
	return NewPersonalizationHandler(svc)
}

func validSettingsBody() string {
	return `{
		"title": "My Status Page",
		"subtitle": "",
		"colors": {
			"bg": "#0B0E13", "surface": "#12151C", "border": "#1F2937",
			"text": "#FFFFFF", "accent": "#22C55E",
			"status_operational": "#22C55E", "status_degraded": "#EAB308",
			"status_partial": "#F97316", "status_major": "#EF4444"
		},
		"announcement": {"enabled": false, "message_md": "", "url": ""},
		"footer_text_md": "",
		"locale": "en",
		"timezone": "",
		"date_format": "relative"
	}`
}

// --- T025: Settings GET/PUT ---

func TestPersonalizationV1_GetSettings_HappyPath(t *testing.T) {
	h := newTestPersoHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status-page/settings", nil)
	rec := httptest.NewRecorder()
	h.HandleGetSettings(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "System Status", resp["title"])
	assert.Equal(t, "en", resp["locale"])
}

func TestPersonalizationV1_PutSettings_HappyPath(t *testing.T) {
	h := newTestPersoHandler(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/status-page/settings", strings.NewReader(validSettingsBody()))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandlePutSettings(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "My Status Page", resp["title"])
	version := resp["version"].(float64)
	assert.Greater(t, version, float64(0))
}

func TestPersonalizationV1_PutSettings_InvalidHex(t *testing.T) {
	h := newTestPersoHandler(t)
	body := `{
		"title": "T",
		"colors": {"bg": "not-hex", "surface": "#12151C", "border": "#1F2937",
			"text": "#FFFFFF", "accent": "#22C55E",
			"status_operational": "#22C55E", "status_degraded": "#EAB308",
			"status_partial": "#F97316", "status_major": "#EF4444"},
		"announcement": {"enabled": false, "message_md": "", "url": ""},
		"footer_text_md": "", "locale": "en", "timezone": "", "date_format": "relative"
	}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/status-page/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandlePutSettings(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// --- T042: Locale / timezone / date_format validation ---

func TestPersonalizationV1_PutSettings_InvalidLocale(t *testing.T) {
	h := newTestPersoHandler(t)
	body := strings.ReplaceAll(validSettingsBody(), `"locale": "en"`, `"locale": "de"`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/status-page/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandlePutSettings(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPersonalizationV1_PutSettings_InvalidTimezone(t *testing.T) {
	h := newTestPersoHandler(t)
	body := strings.ReplaceAll(validSettingsBody(), `"timezone": ""`, `"timezone": "Bogus/Nowhere"`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/status-page/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandlePutSettings(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPersonalizationV1_PutSettings_InvalidDateFormat(t *testing.T) {
	h := newTestPersoHandler(t)
	body := strings.ReplaceAll(validSettingsBody(), `"date_format": "relative"`, `"date_format": "invalid"`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/status-page/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandlePutSettings(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// --- T025: Edition gating ---

func TestPersonalizationV1_EnterpriseGating_PutSettings(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	handler := requireEnterprise(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/status-page/settings", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- T033: Footer link CRUD ---

func TestPersonalizationV1_FooterLinks_Create_HappyPath(t *testing.T) {
	h := newTestPersoHandler(t)
	body := `{"label": "Blog", "url": "https://blog.example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/status-page/footer-links", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleCreateFooterLink(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "Blog", resp["label"])
}

func TestPersonalizationV1_FooterLinks_List(t *testing.T) {
	h := newTestPersoHandler(t)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/status-page/footer-links",
		strings.NewReader(`{"label": "Docs", "url": "https://docs.example.com"}`))
	createReq.Header.Set("Content-Type", "application/json")
	h.HandleCreateFooterLink(httptest.NewRecorder(), createReq)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status-page/footer-links", nil)
	rec := httptest.NewRecorder()
	h.HandleListFooterLinks(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	items := resp["items"].([]interface{})
	assert.Len(t, items, 1)
}

func TestPersonalizationV1_FooterLinks_Delete(t *testing.T) {
	h := newTestPersoHandler(t)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/status-page/footer-links",
		strings.NewReader(`{"label": "Docs", "url": "https://docs.example.com"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	h.HandleCreateFooterLink(createRec, createReq)
	require.Equal(t, http.StatusCreated, createRec.Code)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/status-page/footer-links/1", nil)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	h.HandleDeleteFooterLink(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestPersonalizationV1_FooterLinks_Reorder(t *testing.T) {
	h := newTestPersoHandler(t)

	for _, body := range []string{
		`{"label": "A", "url": "https://a.example.com"}`,
		`{"label": "B", "url": "https://b.example.com"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/status-page/footer-links", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		h.HandleCreateFooterLink(httptest.NewRecorder(), req)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/status-page/footer-links/order",
		strings.NewReader(`{"ids": [2, 1]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleReorderFooterLinks(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestPersonalizationV1_FooterLinks_InvalidURL_JavascriptScheme(t *testing.T) {
	h := newTestPersoHandler(t)
	body := `{"label": "Bad", "url": "javascript:alert(1)"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/status-page/footer-links", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleCreateFooterLink(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPersonalizationV1_FooterLinks_InvalidURL_DataScheme(t *testing.T) {
	h := newTestPersoHandler(t)
	body := `{"label": "Bad", "url": "data:text/html,<h1>xss</h1>"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/status-page/footer-links", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleCreateFooterLink(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPersonalizationV1_FooterLinks_InvalidURL_FtpScheme(t *testing.T) {
	h := newTestPersoHandler(t)
	body := `{"label": "FTP", "url": "ftp://files.example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/status-page/footer-links", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleCreateFooterLink(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// --- T033: FAQ CRUD ---

func TestPersonalizationV1_FAQ_Create_HappyPath(t *testing.T) {
	h := newTestPersoHandler(t)
	body := `{"question": "What is this?", "answer_md": "**Great** monitoring tool."}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/status-page/faq", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleCreateFAQItem(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "What is this?", resp["question"])
	assert.NotEmpty(t, resp["answer_html"])
}

func TestPersonalizationV1_FAQ_List(t *testing.T) {
	h := newTestPersoHandler(t)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/status-page/faq",
		strings.NewReader(`{"question": "Q?", "answer_md": "A."}`))
	createReq.Header.Set("Content-Type", "application/json")
	h.HandleCreateFAQItem(httptest.NewRecorder(), createReq)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status-page/faq", nil)
	rec := httptest.NewRecorder()
	h.HandleListFAQ(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	items := resp["items"].([]interface{})
	assert.Len(t, items, 1)
}

func TestPersonalizationV1_FAQ_Delete(t *testing.T) {
	h := newTestPersoHandler(t)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/status-page/faq",
		strings.NewReader(`{"question": "Q?", "answer_md": "A."}`))
	createReq.Header.Set("Content-Type", "application/json")
	h.HandleCreateFAQItem(httptest.NewRecorder(), createReq)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/status-page/faq/1", nil)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	h.HandleDeleteFAQItem(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestPersonalizationV1_FAQ_Reorder(t *testing.T) {
	h := newTestPersoHandler(t)

	for _, body := range []string{
		`{"question": "Q1?", "answer_md": "A1."}`,
		`{"question": "Q2?", "answer_md": "A2."}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/status-page/faq", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		h.HandleCreateFAQItem(httptest.NewRecorder(), req)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/status-page/faq/order",
		strings.NewReader(`{"ids": [2, 1]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleReorderFAQ(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestPersonalizationV1_FAQ_EnterpriseGating(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	handler := requireEnterprise(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/status-page/faq", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}
