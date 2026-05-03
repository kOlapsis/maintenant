package status

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderAnnouncement_ScriptRemoved(t *testing.T) {
	// Separate paragraphs so the text isn't absorbed into the HTML block
	html, err := RenderAnnouncement("<script>alert(1)</script>\n\nHello")
	require.NoError(t, err)
	assert.NotContains(t, html, "<script")
	assert.Contains(t, strings.TrimSpace(html), "Hello")
}

func TestRenderAnnouncement_IframeRemoved(t *testing.T) {
	html, err := RenderAnnouncement("<iframe src='x'></iframe> text")
	require.NoError(t, err)
	assert.NotContains(t, html, "<iframe")
}

func TestRenderAnnouncement_OnAttributeRemoved(t *testing.T) {
	html, err := RenderAnnouncement("<a href='https://x.com' onclick='bad()'>link</a>")
	require.NoError(t, err)
	assert.NotContains(t, html, "onclick")
}

func TestRenderAnnouncement_InlineStyleRemoved(t *testing.T) {
	html, err := RenderAnnouncement("<p style='color:red'>text</p>")
	require.NoError(t, err)
	assert.NotContains(t, html, "color:red")
}

func TestRenderAnnouncement_ImgNotAllowed(t *testing.T) {
	html, err := RenderAnnouncement("![alt](https://example.com/img.png)")
	require.NoError(t, err)
	assert.NotContains(t, html, "<img")
}

func TestRenderAnnouncement_LinksRewrittenWithTargetAndRel(t *testing.T) {
	html, err := RenderAnnouncement("[click](https://example.com)")
	require.NoError(t, err)
	assert.Contains(t, html, `target="_blank"`)
	assert.Contains(t, html, `rel="noopener noreferrer"`)
}

func TestRenderAnnouncement_AllowedFormattingPreserved(t *testing.T) {
	html, err := RenderAnnouncement("**bold** *italic*")
	require.NoError(t, err)
	assert.Contains(t, html, "<strong>")
	assert.Contains(t, html, "<em>")
}

func TestRenderFooter_ScriptRemoved(t *testing.T) {
	html, err := RenderFooter("<script>alert(1)</script>\n\nFooter text")
	require.NoError(t, err)
	assert.NotContains(t, html, "<script")
	assert.Contains(t, html, "Footer text")
}

func TestRenderFooter_LinksRewritten(t *testing.T) {
	html, err := RenderFooter("[Privacy Policy](https://example.com/privacy)")
	require.NoError(t, err)
	assert.Contains(t, html, `target="_blank"`)
	assert.Contains(t, html, `rel="noopener noreferrer"`)
}

func TestRenderFooter_IframeRemoved(t *testing.T) {
	html, err := RenderFooter("<iframe src='x'></iframe>")
	require.NoError(t, err)
	assert.NotContains(t, html, "<iframe")
}

func TestRenderFAQAnswer_ScriptRemoved(t *testing.T) {
	html, err := RenderFAQAnswer("<script>alert(1)</script> Answer")
	require.NoError(t, err)
	assert.NotContains(t, html, "<script")
}

func TestRenderFAQAnswer_LinksRewritten(t *testing.T) {
	html, err := RenderFAQAnswer("[documentation](https://docs.example.com)")
	require.NoError(t, err)
	assert.Contains(t, html, `target="_blank"`)
	assert.Contains(t, html, `rel="noopener noreferrer"`)
}

func TestRenderFAQAnswer_AllowedFormattingPreserved(t *testing.T) {
	html, err := RenderFAQAnswer("**bold** *italic* `code`\n- list item")
	require.NoError(t, err)
	assert.Contains(t, html, "<strong>")
	assert.Contains(t, html, "<em>")
	assert.Contains(t, html, "<code>")
	assert.Contains(t, html, "<li>")
}

func TestRenderFAQAnswer_InlineStyleRemoved(t *testing.T) {
	html, err := RenderFAQAnswer("<span style='display:none'>hidden</span>")
	require.NoError(t, err)
	assert.NotContains(t, html, "display:none")
}
