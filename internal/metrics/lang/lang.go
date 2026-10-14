// Package lang holds per-language counting rules. Pure data and matchers.
package lang

import (
	"path/filepath"
	"strings"
)

type Spec struct {
	Name     string
	Line     []string    // line comment markers
	Block    [][2]string // block comment open/close
	Quotes   string      // string quote characters
	Keywords map[string]bool
}

func kw(words ...string) map[string]bool {
	m := map[string]bool{}
	for _, w := range words {
		m[w] = true
	}
	return m
}

var (
	cBlock  = [][2]string{{"/*", "*/"}}
	cLike   = kw("if", "for", "while", "case", "catch")
	htmlBlk = [][2]string{{"<!--", "-->"}}
)

var specs = map[string]*Spec{
	".go":     {Name: "go", Line: []string{"//"}, Block: cBlock, Quotes: "\"'`", Keywords: cLike},
	".js":     {Name: "javascript", Line: []string{"//"}, Block: cBlock, Quotes: "\"'`", Keywords: cLike},
	".mjs":    {Name: "javascript", Line: []string{"//"}, Block: cBlock, Quotes: "\"'`", Keywords: cLike},
	".cjs":    {Name: "javascript", Line: []string{"//"}, Block: cBlock, Quotes: "\"'`", Keywords: cLike},
	".jsx":    {Name: "javascript", Line: []string{"//"}, Block: cBlock, Quotes: "\"'`", Keywords: cLike},
	".ts":     {Name: "typescript", Line: []string{"//"}, Block: cBlock, Quotes: "\"'`", Keywords: cLike},
	".tsx":    {Name: "typescript", Line: []string{"//"}, Block: cBlock, Quotes: "\"'`", Keywords: cLike},
	".svelte": {Name: "svelte", Line: []string{"//"}, Block: append([][2]string{{"<!--", "-->"}}, cBlock...), Quotes: "\"'`", Keywords: cLike},
	".vue":    {Name: "vue", Line: []string{"//"}, Block: append([][2]string{{"<!--", "-->"}}, cBlock...), Quotes: "\"'`", Keywords: cLike},
	".py":     {Name: "python", Line: []string{"#"}, Quotes: "\"'", Keywords: kw("if", "elif", "for", "while", "except", "and", "or", "case")},
	".swift":  {Name: "swift", Line: []string{"//"}, Block: cBlock, Quotes: "\"", Keywords: kw("if", "guard", "for", "while", "case", "catch")},
	".kt":     {Name: "kotlin", Line: []string{"//"}, Block: cBlock, Quotes: "\"'", Keywords: kw("if", "for", "while", "when", "catch")},
	".kts":    {Name: "kotlin", Line: []string{"//"}, Block: cBlock, Quotes: "\"'", Keywords: kw("if", "for", "while", "when", "catch")},
	".java":   {Name: "java", Line: []string{"//"}, Block: cBlock, Quotes: "\"'", Keywords: cLike},
	".rs":     {Name: "rust", Line: []string{"//"}, Block: cBlock, Quotes: "\"", Keywords: kw("if", "for", "while", "loop", "match")},
	".c":      {Name: "c", Line: []string{"//"}, Block: cBlock, Quotes: "\"'", Keywords: cLike},
	".h":      {Name: "c", Line: []string{"//"}, Block: cBlock, Quotes: "\"'", Keywords: cLike},
	".cc":     {Name: "cpp", Line: []string{"//"}, Block: cBlock, Quotes: "\"'", Keywords: cLike},
	".cpp":    {Name: "cpp", Line: []string{"//"}, Block: cBlock, Quotes: "\"'", Keywords: cLike},
	".hpp":    {Name: "cpp", Line: []string{"//"}, Block: cBlock, Quotes: "\"'", Keywords: cLike},
	".rb":     {Name: "ruby", Line: []string{"#"}, Block: [][2]string{{"=begin", "=end"}}, Quotes: "\"'", Keywords: kw("if", "elsif", "unless", "while", "until", "when", "rescue", "and", "or")},
	".php":    {Name: "php", Line: []string{"//", "#"}, Block: cBlock, Quotes: "\"'", Keywords: kw("if", "elseif", "for", "foreach", "while", "case", "catch")},
	".dart":   {Name: "dart", Line: []string{"//"}, Block: cBlock, Quotes: "\"'", Keywords: cLike},
	".tf":     {Name: "terraform", Line: []string{"#", "//"}, Block: cBlock, Quotes: "\"", Keywords: kw()},
	".hcl":    {Name: "terraform", Line: []string{"#", "//"}, Block: cBlock, Quotes: "\"", Keywords: kw()},
	".sh":     {Name: "shell", Line: []string{"#"}, Quotes: "\"'", Keywords: kw("if", "elif", "for", "while", "case", "until")},
	".bash":   {Name: "shell", Line: []string{"#"}, Quotes: "\"'", Keywords: kw("if", "elif", "for", "while", "case", "until")},
	".zsh":    {Name: "shell", Line: []string{"#"}, Quotes: "\"'", Keywords: kw("if", "elif", "for", "while", "case", "until")},
	".sql":    {Name: "sql", Line: []string{"--"}, Block: cBlock, Quotes: "'", Keywords: kw("when")},
	".css":    {Name: "css", Block: cBlock, Quotes: "\"'", Keywords: kw()},
	".scss":   {Name: "css", Line: []string{"//"}, Block: cBlock, Quotes: "\"'", Keywords: kw()},
	".html":   {Name: "html", Block: htmlBlk, Keywords: kw()},
	".md":     {Name: "markdown", Block: htmlBlk, Keywords: kw()},
}

// ForPath returns the spec for a file by extension.
func ForPath(path string) (*Spec, bool) {
	s, ok := specs[strings.ToLower(filepath.Ext(path))]
	return s, ok
}

var lockfiles = map[string]bool{
	"package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true, "go.sum": true,
	"cargo.lock": true, "gemfile.lock": true, "poetry.lock": true, "composer.lock": true,
}

// IsExcludedName reports files that would distort size: lockfiles, minified.
func IsExcludedName(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return lockfiles[base] || strings.Contains(base, ".min.")
}

// IsTest reports test files by path convention.
func IsTest(path string) bool {
	p := "/" + strings.ToLower(filepath.ToSlash(path))
	base := p[strings.LastIndex(p, "/")+1:]
	switch {
	case strings.HasSuffix(base, "_test.go"), strings.Contains(base, ".test."), strings.Contains(base, ".spec."),
		strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py"), strings.HasSuffix(base, "_test.py"):
		return true
	}
	return strings.Contains(p, "/__tests__/") || strings.Contains(p, "/test/") || strings.Contains(p, "/tests/")
}
