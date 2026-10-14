// Package count classifies lines and estimates complexity for one file. Pure.
package count

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/halalgami/CodingAgentCommander/internal/metrics/lang"
)

type FileStats struct {
	Lang                             string
	Code, Comment, Blank, Complexity int
	Todos                            int
	Test, Excluded                   bool
}

var todoRe = regexp.MustCompile(`\b(TODO|FIXME|HACK|XXX)\b`)

func excluded(name string, src []byte) bool {
	if lang.IsExcludedName(name) {
		return true
	}
	head := src
	if len(head) > 8000 {
		head = head[:8000]
	}
	if bytes.IndexByte(head, 0) >= 0 {
		return true
	}
	if len(head) > 512 {
		head = head[:512]
	}
	return bytes.Contains(head, []byte("Code generated")) && bytes.Contains(head, []byte("DO NOT EDIT"))
}

// Count classifies every line of src as code, comment or blank and estimates
// complexity as 1 + decision points found in code text.
func Count(name string, src []byte) FileStats {
	st := FileStats{Lang: "other", Complexity: 1, Test: lang.IsTest(name)}
	if excluded(name, src) {
		st.Excluded = true
		return st
	}
	spec, known := lang.ForPath(name)
	if known {
		st.Lang = spec.Name
	}
	text := string(src)
	if text == "" {
		return st
	}
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	inBlock := ""
	decisions := 0
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			st.Blank++
			continue
		}
		if !known {
			st.Code++
			continue
		}
		code, comment := split(line, spec, &inBlock)
		if strings.TrimSpace(code) != "" {
			st.Code++
			decisions += decisionsIn(code, spec)
		} else {
			st.Comment++
		}
		if todoRe.MatchString(comment) {
			st.Todos++
		}
	}
	st.Complexity = 1 + decisions
	return st
}

// split separates one line into code text (string contents blanked) and
// comment text, carrying block-comment state across lines.
func split(line string, sp *lang.Spec, inBlock *string) (code, comment string) {
	var cb, mb strings.Builder
	var quote byte
	for i := 0; i < len(line); {
		if *inBlock != "" {
			if strings.HasPrefix(line[i:], *inBlock) {
				i += len(*inBlock)
				*inBlock = ""
				continue
			}
			mb.WriteByte(line[i])
			i++
			continue
		}
		c := line[i]
		if quote != 0 {
			if c == '\\' {
				i += 2
				continue
			}
			if c == quote {
				quote = 0
			}
			i++
			continue
		}
		if strings.IndexByte(sp.Quotes, c) >= 0 {
			quote = c
			cb.WriteByte('"')
			i++
			continue
		}
		if m := prefixAny(line[i:], sp.Line); m {
			mb.WriteString(line[i:])
			break
		}
		if open, close := blockAt(line[i:], sp.Block); open != "" {
			*inBlock = close
			i += len(open)
			continue
		}
		cb.WriteByte(c)
		i++
	}
	return cb.String(), mb.String()
}

func prefixAny(s string, markers []string) bool {
	for _, m := range markers {
		if strings.HasPrefix(s, m) {
			return true
		}
	}
	return false
}

func blockAt(s string, blocks [][2]string) (string, string) {
	for _, b := range blocks {
		if strings.HasPrefix(s, b[0]) {
			return b[0], b[1]
		}
	}
	return "", ""
}

func isWord(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func decisionsIn(code string, sp *lang.Spec) int {
	n := strings.Count(code, "&&") + strings.Count(code, "||")
	for i := 0; i < len(code); {
		if !isWord(code[i]) {
			i++
			continue
		}
		j := i
		for j < len(code) && isWord(code[j]) {
			j++
		}
		if sp.Keywords[code[i:j]] {
			n++
		}
		i = j
	}
	return n
}
