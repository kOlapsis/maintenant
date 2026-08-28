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

package store

import (
	"strconv"
	"strings"
)

// rebindPostgres rewrites `?` placeholders as `$1..$n`. A `?` inside a string
// literal ('...' with '' escapes), a quoted identifier ("..."), a line comment
// (-- ...) or a block comment (/* ... */) is left untouched, as is a doubled
// `??` (not a placeholder on either engine).
func rebindPostgres(query string) string {
	var b strings.Builder
	b.Grow(len(query) + 16)
	n := 0
	i := 0
	for i < len(query) {
		c := query[i]
		switch {
		case c == '\'' || c == '"':
			// Copy the quoted region verbatim. A doubled quote is an escape
			// and does not close the region.
			quote := c
			b.WriteByte(c)
			i++
			for i < len(query) {
				b.WriteByte(query[i])
				if query[i] == quote {
					if i+1 < len(query) && query[i+1] == quote {
						i++
						b.WriteByte(query[i])
					} else {
						i++
						break
					}
				}
				i++
			}
		case c == '-' && i+1 < len(query) && query[i+1] == '-':
			// Line comment: copy up to and including the newline.
			end := strings.IndexByte(query[i:], '\n')
			if end < 0 {
				b.WriteString(query[i:])
				i = len(query)
			} else {
				b.WriteString(query[i : i+end+1])
				i += end + 1
			}
		case c == '/' && i+1 < len(query) && query[i+1] == '*':
			end := strings.Index(query[i+2:], "*/")
			if end < 0 {
				b.WriteString(query[i:])
				i = len(query)
			} else {
				b.WriteString(query[i : i+2+end+2])
				i += 2 + end + 2
			}
		case c == '?':
			if i+1 < len(query) && query[i+1] == '?' {
				b.WriteString("??")
				i += 2
			} else {
				n++
				b.WriteByte('$')
				b.WriteString(strconv.Itoa(n))
				i++
			}
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}
