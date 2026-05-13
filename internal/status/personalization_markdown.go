package status

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
)

var announcementPolicy = buildAnnouncementPolicy()
var footerPolicy = buildFooterPolicy()
var faqPolicy = buildFAQPolicy()

func buildAnnouncementPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "strong", "em")
	p.AllowAttrs("href").OnElements("a")
	p.RequireNoFollowOnLinks(false)
	p.AllowRelativeURLs(false)
	p.AllowURLSchemes("http", "https")
	p.RequireParseableURLs(true)
	return p
}

func buildFooterPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "strong", "em", "ul", "ol", "li", "code")
	p.AllowAttrs("href").OnElements("a")
	p.RequireNoFollowOnLinks(false)
	p.AllowRelativeURLs(false)
	p.AllowURLSchemes("http", "https")
	p.RequireParseableURLs(true)
	return p
}

func buildFAQPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "strong", "em", "ul", "ol", "li", "code", "h3", "h4", "pre", "blockquote")
	p.AllowAttrs("href").OnElements("a")
	p.RequireNoFollowOnLinks(false)
	p.AllowRelativeURLs(false)
	p.AllowURLSchemes("http", "https")
	p.RequireParseableURLs(true)
	return p
}

func renderAndSanitize(md string, policy *bluemonday.Policy) (string, error) {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(md), &buf); err != nil {
		return "", fmt.Errorf("markdown render: %w", err)
	}
	sanitized := policy.Sanitize(buf.String())
	// Rewrite all <a> tags to add target="_blank" rel="noopener noreferrer"
	sanitized = rewriteLinks(sanitized)
	return sanitized, nil
}

// rewriteLinks adds target="_blank" rel="noopener noreferrer" to every <a href=...> tag.
func rewriteLinks(html string) string {
	const open = `<a href="`
	if !strings.Contains(html, "<a ") {
		return html
	}

	var sb strings.Builder
	rest := html
	for {
		idx := strings.Index(rest, open)
		if idx == -1 {
			sb.WriteString(rest)
			break
		}
		sb.WriteString(rest[:idx])
		// find end of opening tag
		tagStart := rest[idx:]
		end := strings.Index(tagStart, ">")
		if end == -1 {
			sb.WriteString(rest[idx:])
			break
		}
		tag := tagStart[:end+1]
		// Strip any existing target/rel from the tag before adding ours
		tag = strings.ReplaceAll(tag, ` target="_blank"`, "")
		tag = strings.ReplaceAll(tag, ` rel="noopener noreferrer"`, "")
		// Insert before closing >
		tag = tag[:len(tag)-1] + ` target="_blank" rel="noopener noreferrer">`
		sb.WriteString(tag)
		rest = rest[idx+end+1:]
	}
	return sb.String()
}

func RenderAnnouncement(md string) (string, error) {
	return renderAndSanitize(md, announcementPolicy)
}

func RenderFooter(md string) (string, error) {
	return renderAndSanitize(md, footerPolicy)
}

func RenderFAQAnswer(md string) (string, error) {
	return renderAndSanitize(md, faqPolicy)
}
