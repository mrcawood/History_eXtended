package query

import (
	"strings"
	"unicode"
)

// stopwords: common question/location words that rarely help FTS.
var stopwords = map[string]bool{
	"where": true, "is": true, "the": true, "a": true, "an": true, "located": true,
	"in": true, "on": true, "at": true, "to": true, "of": true, "for": true,
	"what": true, "how": true, "when": true, "which": true, "who": true, "why": true,
	"did": true, "do": true, "does": true, "can": true,
	"with": true, "from": true, "by": true, "my": true, "me": true, "i": true,
	"it": true, "its": true, "as": true, "or": true, "and": true, "but": true,
}

// ExtractKeywords returns tokens from a natural-language query for FTS.
// Lowercases, strips punctuation, tokenizes on whitespace, removes stopwords,
// keeps tokens with letters/digits and length >= 2 (or repo-like single chars).
func ExtractKeywords(query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	// Tokenize: split on whitespace and punctuation, keep alnum
	var tokens []string
	var buf strings.Builder
	for _, r := range query {
		// Split on space and punct, but keep _ and - inside tokens (paths, repo names)
		isSplit := unicode.IsSpace(r) || (unicode.IsPunct(r) && r != '_' && r != '-')
		if isSplit {
			if buf.Len() > 0 {
				tokens = append(tokens, buf.String())
				buf.Reset()
			}
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			buf.WriteRune(r)
		}
	}
	if buf.Len() > 0 {
		tokens = append(tokens, buf.String())
	}
	// Dedupe preserving order
	seen := make(map[string]bool)
	var out []string
	for _, t := range tokens {
		if seen[t] {
			continue
		}
		seen[t] = true
		if stopwords[t] {
			continue
		}
		// Keep tokens with length >= 2, or single-char if alnum (repo names like "x")
		keep := len(t) >= 2
		if !keep && len(t) == 1 && unicode.IsLetter(rune(t[0])) {
			keep = true
		}
		if keep {
			out = append(out, t)
		}
	}
	return out
}

// BuildFTSQuery builds an FTS5 query string from keywords.
// - Single keyword: use as-is.
// - Multiple: OR across tokens to avoid over-constraining.
func BuildFTSQuery(keywords []string) string {
	if len(keywords) == 0 {
		return ""
	}
	// Escape FTS5 special chars: " - ( ) for phrase/grouping
	escaped := make([]string, len(keywords))
	for i, k := range keywords {
		k = strings.ReplaceAll(k, "\"", "\"\"")
		escaped[i] = k
	}
	if len(escaped) == 1 {
		return escaped[0]
	}
	return strings.Join(escaped, " OR ")
}
