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
package channels

import (
	"encoding/json"
	"fmt"

	"github.com/kolapsis/maintenant/internal/alert"
)

func formatSlackPayload(eventType string, a *alert.Alert) ([]byte, error) {
	emoji := alert.SeverityEmoji(a.Severity)
	title := fmt.Sprintf("%s *%s*", emoji, alert.EventTitle(eventType, a))

	fields := fmt.Sprintf(
		"*Source:* %s  |  *Severity:* %s  |  *Entity:* %s\n%s",
		a.Source, a.Severity, a.EntityName, a.Message,
	)

	blocks := []map[string]interface{}{
		{
			"type": "section",
			"text": map[string]string{
				"type": "mrkdwn",
				"text": title,
			},
		},
		{
			"type": "section",
			"text": map[string]string{
				"type": "mrkdwn",
				"text": fields,
			},
		},
	}

	if a.Source == "update" {
		if details := alert.ParseAlertDetails(a.Details); details != nil {
			if cmd, ok := details["update_command"].(string); ok && cmd != "" {
				blocks = append(blocks, map[string]interface{}{
					"type": "section",
					"text": map[string]string{
						"type": "mrkdwn",
						"text": fmt.Sprintf("*Update command:*\n```%s```", cmd),
					},
				})
			}
			if cmd, ok := details["rollback_command"].(string); ok && cmd != "" {
				blocks = append(blocks, map[string]interface{}{
					"type": "section",
					"text": map[string]string{
						"type": "mrkdwn",
						"text": fmt.Sprintf("*Rollback command:*\n```%s```", cmd),
					},
				})
			}
		}
	}

	payload := map[string]interface{}{
		"blocks": blocks,
	}
	return json.Marshal(payload)
}

func formatTeamsPayload(eventType string, a *alert.Alert) ([]byte, error) {
	facts := []map[string]string{
		{"name": "Source", "value": a.Source},
		{"name": "Severity", "value": a.Severity},
		{"name": "Entity", "value": a.EntityName},
		{"name": "Type", "value": a.AlertType},
	}

	if a.Source == "update" {
		if details := alert.ParseAlertDetails(a.Details); details != nil {
			if cmd, ok := details["update_command"].(string); ok && cmd != "" {
				facts = append(facts, map[string]string{"name": "Update Command", "value": "`" + cmd + "`"})
			}
			if cmd, ok := details["rollback_command"].(string); ok && cmd != "" {
				facts = append(facts, map[string]string{"name": "Rollback Command", "value": "`" + cmd + "`"})
			}
		}
	}

	payload := map[string]interface{}{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"themeColor": severityHexColor(a.Severity),
		"title":      alert.EventTitle(eventType, a),
		"sections": []map[string]interface{}{
			{
				"activityTitle": a.Message,
				"facts":         facts,
			},
		},
	}
	return json.Marshal(payload)
}

func severityHexColor(severity string) string {
	switch severity {
	case alert.SeverityCritical:
		return "EF4444"
	case alert.SeverityWarning:
		return "F59E0B"
	default:
		return "22C55E"
	}
}
