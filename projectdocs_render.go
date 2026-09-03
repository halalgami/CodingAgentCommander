package main

// Doc viewer, rendering half. This runs in Go rather than in the webview
// deliberately (spec R2.2): the webview is where this app's Go bindings live
// on `window`, the frontend therefore parses nothing, and these assertions run
// in the suite CI actually executes — CI runs Go and the node tests, never
// Playwright.
//
// Two layers live here. goldmark does not emit raw HTML or dangerous URLs
// unless given html.WithUnsafe(), which it is not. bluemonday then sanitizes
// the result anyway, because a renderer's safety flag is a promise and a
// sanitizer is a check.

import (
	"bytes"
	"path/filepath"
	"strings"
	"sync"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
	"golang.org/x/net/html"
)

// docChromaStyle is the highlighting theme. Dark, to match the frame CSS.
const docChromaStyle = "github-dark"

var docMarkdownExt = map[string]bool{".md": true, ".markdown": true}

// DocLink is one link found in a document, href exactly as written. Go makes
// no judgement about it; the frontend classifies (spec R2.4).
type DocLink struct {
	Text string `json:"text"`
	Href string `json:"href"`
}

// DocRender is everything the viewer needs for one document.
type DocRender struct {
	HTML  string    `json:"html"` // sanitized, ready for srcdoc
	CSS   string    `json:"css"`  // highlighting stylesheet, "" when unused
	Kind  string    `json:"kind"` // "markdown" | "text"
	Lang  string    `json:"lang"` // chroma lexer name, "" for markdown
	Links []DocLink `json:"links"`
}

var (
	mdOnce sync.Once
	md     goldmark.Markdown

	policyOnce sync.Once
	policy     *bluemonday.Policy

	cssOnce sync.Once
	cssText string
)

func docMarkdown() goldmark.Markdown {
	mdOnce.Do(func() {
		md = goldmark.New(
			// GFM covers what the corpus uses: tables, task lists,
			// strikethrough, autolinks.
			goldmark.WithExtensions(
				extension.GFM,
				highlighting.NewHighlighting(
					highlighting.WithStyle(docChromaStyle),
					// Classes, not inline styles: the sanitizer forbids a
					// style attribute, so inline highlighting would be
					// stripped and every fence would render unstyled.
					highlighting.WithFormatOptions(chromahtml.WithClasses(true)),
				),
			),
			// NOTE: no html.WithUnsafe(). Raw HTML in a document is not
			// rendered, which is the first of the two layers here.
		)
	})
	return md
}

// docSanitizer is an allowlist: structure and highlighting classes, nothing
// else. No href (spec R2.2 — with no href there is nothing in the frame to
// navigate to, so link inertness stops depending on engine behaviour), no src,
// no style, no id, no event attributes.
//
// AllowAttrs(...).OnElements("input") already registers "input" in the
// policy's element table (confirmed by reading bluemonday's policy.go: an
// OnElements call creates the elsAndAttrs entry itself), so a trailing
// AllowElements("input") is redundant. It is kept anyway — it costs nothing,
// documents intent for the next reader, and does not depend on that detail
// staying true across a bluemonday upgrade.
func docSanitizer() *bluemonday.Policy {
	policyOnce.Do(func() {
		p := bluemonday.NewPolicy()
		p.AllowElements(
			"p", "h1", "h2", "h3", "h4", "h5", "h6",
			"ul", "ol", "li", "dl", "dt", "dd",
			"pre", "code", "blockquote", "hr", "br",
			"em", "strong", "del", "sup", "sub", "span", "div",
			"table", "thead", "tbody", "tfoot", "tr", "th", "td", "caption",
			// "a" is allowed as an ELEMENT while href is never allowed as an
			// attribute. An anchor with no href navigates nothing and is not
			// even focusable, so it is as inert as bare text — but it keeps
			// link text visually distinguishable, which in a plan or spec
			// document is most of the prose. Stripping the element outright
			// (the first version of this policy) made "see [the spec](...)"
			// render identically to ordinary text, hiding from the reader that
			// there was a target at all. The targets are listed in the drawer.
			"a",
		)
		// Task lists render as a disabled checkbox. Allowed narrowly, because
		// a checklist is most of what a plan document IS.
		p.AllowAttrs("type").Matching(bluemonday.Paragraph).OnElements("input")
		p.AllowAttrs("checked", "disabled").OnElements("input")
		p.AllowElements("input")
		// Highlighting is class-driven, so classes are the one attribute that
		// must survive.
		p.AllowAttrs("class").Matching(bluemonday.SpaceSeparatedTokens).
			OnElements("span", "code", "pre", "div", "table", "tr", "td", "th", "li", "p")
		// An anchor is the ONE element AllowElements is not enough for: with no
		// attribute surviving the policy, bluemonday drops the element too, so
		// link text collapsed into prose. Verified against v1.0.27 with a
		// standalone probe: AllowElements("p","a") keeps <p> and discards <a>,
		// while AllowNoAttrs().OnElements("a") keeps <a>. Nothing here ever
		// allows href, so the anchor stays inert.
		p.AllowNoAttrs().OnElements("a")
		p.AllowAttrs("align").Matching(bluemonday.Paragraph).OnElements("th", "td")
		policy = p
	})
	return policy
}

// docChromaCSS is the stylesheet for the highlighting classes, generated once
// from a static style name. It is trusted output, and it goes into the frame's
// own <style>, not through the sanitizer.
func docChromaCSS() string {
	cssOnce.Do(func() {
		style := styles.Get(docChromaStyle)
		if style == nil {
			style = styles.Fallback
		}
		var b bytes.Buffer
		f := chromahtml.New(chromahtml.WithClasses(true))
		if err := f.WriteCSS(&b, style); err != nil {
			cssText = ""
			return
		}
		cssText = b.String()
	})
	return cssText
}

// renderDoc renders one document. It never returns an error: an unparseable
// document is not a thing markdown has, and a lexer miss falls back to plain
// text rather than failing the read the user asked for.
//
// Markdown is parsed and rendered as two separate steps — mirroring what
// goldmark's own Convert does internally (parse, then render the same AST) —
// rather than through Convert directly. That split is load-bearing, not
// stylistic: goldmark's HTML renderer refuses to emit a "dangerous" URL
// (javascript:, vbscript:, file:, most data:) as an href WHENEVER
// html.WithUnsafe is not set (verified by reading
// renderer/html/html.go — IsDangerousURL and its three call sites are gated
// on `r.Unsafe`, not on anything raw-HTML-specific). That is a second,
// independent reason a javascript: link's href never reaches the sanitized
// HTML — on top of bluemonday dropping every href regardless of scheme — but
// it also means the ORIGINAL plan (extracting links by re-tokenizing the
// rendered HTML) can never recover that href: by the time HTML exists, this
// document's own javascript: link has already been rewritten to `href=""`
// and extraction would silently lose it. Reading it from the AST's Link/
// AutoLink nodes instead — before the renderer ever touches it — gets the
// destination exactly as parsed, independent of the renderer's own safety
// net, which is what the frontend's classifier (spec R2.4) needs to see.
func renderDoc(rel, src string) DocRender {
	if docMarkdownExt[strings.ToLower(filepath.Ext(rel))] {
		source := []byte(src)
		doc := docMarkdown().Parser().Parse(text.NewReader(source))
		var raw bytes.Buffer
		if err := docMarkdown().Renderer().Render(&raw, source, doc); err != nil {
			// goldmark only errors on a writer failure, which a bytes.Buffer
			// cannot have — but returning the source as text beats a panic.
			return renderAsText(rel, src)
		}
		return DocRender{
			HTML:  docSanitizer().Sanitize(raw.String()),
			CSS:   docChromaCSS(),
			Kind:  "markdown",
			Links: extractLinks(doc, source),
		}
	}
	return renderAsText(rel, src)
}

func renderAsText(rel, src string) DocRender {
	lexer := lexers.Match(filepath.Base(rel))
	if lexer == nil {
		lexer = lexers.Analyse(src)
	}
	lang := ""
	var out bytes.Buffer
	if lexer != nil {
		lang = lexer.Config().Name
		style := styles.Get(docChromaStyle)
		if style == nil {
			style = styles.Fallback
		}
		it, err := lexer.Tokenise(nil, src)
		if err == nil {
			f := chromahtml.New(chromahtml.WithClasses(true), chromahtml.WithLineNumbers(true))
			if err := f.Format(&out, style, it); err != nil {
				out.Reset()
			}
		}
	}
	if out.Len() == 0 {
		// No lexer, or highlighting failed: emit the text escaped inside a
		// pre, which the sanitizer will pass through untouched.
		out.WriteString("<pre><code>")
		out.WriteString(html.EscapeString(src))
		out.WriteString("</code></pre>")
	}
	return DocRender{
		HTML: docSanitizer().Sanitize(out.String()),
		CSS:  docChromaCSS(),
		Kind: "text",
		Lang: lang,
	}
}

// extractLinks reads links out of the PARSED AST, before the HTML renderer
// ever runs — not out of the rendered HTML, and that is deliberate (see the
// long comment on renderDoc). Hrefs are returned verbatim, including a
// javascript: one — the frontend's classifier is what decides such a link is
// inert, and hiding it here would hide it from the classifier's tests too.
func extractLinks(doc gast.Node, source []byte) []DocLink {
	var links []DocLink
	_ = gast.Walk(doc, func(n gast.Node, entering bool) (gast.WalkStatus, error) {
		if !entering {
			return gast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *gast.Link:
			links = append(links, DocLink{
				Text: strings.TrimSpace(string(v.Text(source))),
				Href: string(v.Destination),
			})
		case *gast.AutoLink:
			links = append(links, DocLink{
				Text: strings.TrimSpace(string(v.Text(source))),
				Href: string(v.URL(source)),
			})
		}
		return gast.WalkContinue, nil
	})
	return links
}
