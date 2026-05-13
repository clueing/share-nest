package preview

import (
	"fmt"
	"html"
	"html/template"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	fencePattern        = regexp.MustCompile("^```\\s*([A-Za-z0-9_+-]*)\\s*$")
	headingPattern      = regexp.MustCompile("^(#{1,6})\\s+(.*)$")
	unorderedListPattern = regexp.MustCompile(`^[-*+]\s+(.*)$`)
	orderedListPattern  = regexp.MustCompile(`^\d+\.\s+(.*)$`)
	linkPattern         = regexp.MustCompile(`\[(.*?)\]\((.*?)\)`)
	boldPattern         = regexp.MustCompile(`\*\*(.+?)\*\*`)
	italicPattern       = regexp.MustCompile(`\*(.+?)\*`)
	inlineCodePattern   = regexp.MustCompile("`([^`]+)`")
)

func IsMarkdown(name, mimeType string) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	return ext == "md" || strings.Contains(mimeType, "markdown")
}

func CodeLanguage(name, mimeType string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	switch ext {
	case "md":
		return "markdown"
	case "json":
		return "json"
	case "yaml", "yml":
		return "yaml"
	case "xml":
		return "xml"
	case "csv", "tsv":
		return "csv"
	case "go":
		return "go"
	case "js", "mjs", "cjs", "jsx":
		return "javascript"
	case "ts", "tsx":
		return "typescript"
	case "py":
		return "python"
	case "java":
		return "java"
	case "c", "h":
		return "c"
	case "cc", "cpp", "cxx", "hpp":
		return "cpp"
	case "rs":
		return "rust"
	case "sh", "bash", "zsh":
		return "bash"
	case "sql":
		return "sql"
	case "html", "htm":
		return "html"
	case "css":
		return "css"
	case "ini", "conf", "cfg", "env":
		return "ini"
	case "toml":
		return "toml"
	case "svg":
		return "xml"
	case "txt", "log":
		return "text"
	}

	if strings.Contains(strings.ToLower(mimeType), "json") {
		return "json"
	}
	if strings.Contains(strings.ToLower(mimeType), "xml") {
		return "xml"
	}
	if strings.HasPrefix(strings.ToLower(mimeType), "text/") {
		return "text"
	}
	return "text"
}

func LanguageLabel(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "javascript":
		return "JavaScript"
	case "typescript":
		return "TypeScript"
	case "python":
		return "Python"
	case "bash":
		return "Bash"
	case "json":
		return "JSON"
	case "yaml":
		return "YAML"
	case "xml":
		return "XML"
	case "html":
		return "HTML"
	case "css":
		return "CSS"
	case "sql":
		return "SQL"
	case "go":
		return "Go"
	case "java":
		return "Java"
	case "c":
		return "C"
	case "cpp":
		return "C++"
	case "rust":
		return "Rust"
	case "toml":
		return "TOML"
	case "ini":
		return "INI"
	case "markdown":
		return "Markdown"
	case "csv":
		return "CSV"
	case "text":
		return "Text"
	default:
		if language == "" {
			return "Text"
		}
		return strings.ToUpper(language[:1]) + language[1:]
	}
}

func RenderMarkdown(source string) template.HTML {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	var out strings.Builder
	out.WriteString(`<div class="markdown-preview" data-markdown-body="true">`)

	var paragraph []string
	var listItems []string
	listKind := ""
	inBlockquote := false
	var quoteLines []string
	inFence := false
	fenceLang := ""
	var fenceLines []string

	flushParagraph := func() {
		if len(paragraph) == 0 {
			return
		}
		text := strings.Join(paragraph, " ")
		out.WriteString("<p>")
		out.WriteString(renderInlineMarkdown(text))
		out.WriteString("</p>")
		paragraph = nil
	}

	flushList := func() {
		if len(listItems) == 0 {
			return
		}
		tag := "ul"
		if listKind == "ol" {
			tag = "ol"
		}
		out.WriteString("<" + tag + ` class="markdown-list">`)
		for _, item := range listItems {
			out.WriteString("<li>")
			out.WriteString(renderInlineMarkdown(item))
			out.WriteString("</li>")
		}
		out.WriteString("</" + tag + ">")
		listItems = nil
		listKind = ""
	}

	flushQuote := func() {
		if !inBlockquote {
			return
		}
		out.WriteString(`<blockquote class="markdown-quote">`)
		for _, line := range quoteLines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			out.WriteString("<p>")
			out.WriteString(renderInlineMarkdown(line))
			out.WriteString("</p>")
		}
		out.WriteString("</blockquote>")
		quoteLines = nil
		inBlockquote = false
	}

	flushFence := func() {
		if !inFence {
			return
		}
		language := normalizeFenceLanguage(fenceLang)
		rendered := RenderCodeHTML(strings.Join(fenceLines, "\n"), language)
		out.WriteString(`<div class="code-block-shell">`)
		out.WriteString(`<div class="code-block-head"><span class="code-language-chip">`)
		out.WriteString(html.EscapeString(LanguageLabel(language)))
		out.WriteString(`</span></div>`)
		out.WriteString(`<div class="markdown-code-shell">`)
		out.WriteString(string(rendered))
		out.WriteString(`</div></div>`)
		fenceLines = nil
		fenceLang = ""
		inFence = false
	}

	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, " \t")

		if inFence {
			if strings.TrimSpace(line) == "```" {
				flushFence()
				continue
			}
			fenceLines = append(fenceLines, rawLine)
			continue
		}

		if matches := fencePattern.FindStringSubmatch(strings.TrimSpace(line)); len(matches) == 2 {
			flushParagraph()
			flushList()
			flushQuote()
			inFence = true
			fenceLang = matches[1]
			fenceLines = nil
			continue
		}

		if strings.TrimSpace(line) == "" {
			flushParagraph()
			flushList()
			flushQuote()
			continue
		}

		if matches := headingPattern.FindStringSubmatch(line); len(matches) == 3 {
			flushParagraph()
			flushList()
			flushQuote()
			level := len(matches[1])
			out.WriteString("<h" + strconv.Itoa(level) + ">")
			out.WriteString(renderInlineMarkdown(matches[2]))
			out.WriteString("</h" + strconv.Itoa(level) + ">")
			continue
		}

		if strings.HasPrefix(strings.TrimSpace(line), ">") {
			flushParagraph()
			flushList()
			inBlockquote = true
			quoteLines = append(quoteLines, strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), ">")))
			continue
		}
		flushQuote()

		if matches := unorderedListPattern.FindStringSubmatch(strings.TrimSpace(line)); len(matches) == 2 {
			flushParagraph()
			if listKind != "" && listKind != "ul" {
				flushList()
			}
			listKind = "ul"
			listItems = append(listItems, matches[1])
			continue
		}

		if matches := orderedListPattern.FindStringSubmatch(strings.TrimSpace(line)); len(matches) == 2 {
			flushParagraph()
			if listKind != "" && listKind != "ol" {
				flushList()
			}
			listKind = "ol"
			listItems = append(listItems, matches[1])
			continue
		}

		flushList()
		paragraph = append(paragraph, strings.TrimSpace(line))
	}

	flushParagraph()
	flushList()
	flushQuote()
	flushFence()
	out.WriteString("</div>")
	return template.HTML(out.String())
}

func normalizeFenceLanguage(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "js", "node", "javascript":
		return "javascript"
	case "ts", "typescript":
		return "typescript"
	case "py", "python":
		return "python"
	case "shell", "bash", "sh", "zsh":
		return "bash"
	case "html":
		return "html"
	case "css":
		return "css"
	case "json":
		return "json"
	case "yaml", "yml":
		return "yaml"
	case "xml":
		return "xml"
	case "sql":
		return "sql"
	case "go":
		return "go"
	case "rust", "rs":
		return "rust"
	case "java":
		return "java"
	case "c":
		return "c"
	case "cpp", "c++", "cc":
		return "cpp"
	default:
		if value == "" {
			return "text"
		}
		return value
	}
}

func renderInlineMarkdown(source string) string {
	codeTokens := make([]string, 0, 8)
	stashed := inlineCodePattern.ReplaceAllStringFunc(source, func(match string) string {
		groups := inlineCodePattern.FindStringSubmatch(match)
		if len(groups) != 2 {
			return match
		}
		token := fmt.Sprintf("@@INLINECODE%d@@", len(codeTokens))
		codeTokens = append(codeTokens, `<code>`+html.EscapeString(groups[1])+`</code>`)
		return token
	})

	escaped := html.EscapeString(stashed)
	escaped = boldPattern.ReplaceAllString(escaped, `<strong>$1</strong>`)
	escaped = italicPattern.ReplaceAllString(escaped, `<em>$1</em>`)
	escaped = linkPattern.ReplaceAllStringFunc(escaped, func(match string) string {
		groups := linkPattern.FindStringSubmatch(html.UnescapeString(match))
		if len(groups) != 3 {
			return match
		}
		href := sanitizeMarkdownURL(groups[2])
		if href == "" {
			return html.EscapeString(groups[1])
		}
		return `<a href="` + html.EscapeString(href) + `" target="_blank" rel="noreferrer noopener">` + html.EscapeString(groups[1]) + `</a>`
	})
	for index, token := range codeTokens {
		escaped = strings.ReplaceAll(escaped, fmt.Sprintf("@@INLINECODE%d@@", index), token)
	}
	return strings.ReplaceAll(escaped, "\n", "<br>")
}

func sanitizeMarkdownURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if parsed.Scheme == "" || parsed.Scheme == "http" || parsed.Scheme == "https" || parsed.Scheme == "mailto" {
		return raw
	}
	return ""
}
