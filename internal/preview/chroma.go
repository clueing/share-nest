package preview

import (
	"bytes"
	"html/template"
	"sync"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

var (
	chromaFormatter = chromahtml.New(
		chromahtml.WithClasses(true),
		chromahtml.TabWidth(2),
	)
	chromaStyle = func() *chroma.Style {
		if style := styles.Get("dracula"); style != nil {
			return style
		}
		return styles.Fallback
	}()
	chromaCSSOnce sync.Once
	chromaCSS     template.CSS
)

func ThemeCSS() template.CSS {
	chromaCSSOnce.Do(func() {
		var buf bytes.Buffer
		if err := chromaFormatter.WriteCSS(&buf, chromaStyle); err == nil {
			chromaCSS = template.CSS(buf.String())
		}
	})
	return chromaCSS
}

func RenderCodeHTML(source, language string) template.HTML {
	lexer := lexers.Get(language)
	if lexer == nil {
		lexer = lexers.Analyse(source)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	iterator, err := lexer.Tokenise(nil, source)
	if err != nil {
		return template.HTML("<pre class=\"chroma\"><code></code></pre>")
	}

	var buf bytes.Buffer
	if err := chromaFormatter.Format(&buf, chromaStyle, iterator); err != nil {
		return template.HTML("<pre class=\"chroma\"><code></code></pre>")
	}
	return template.HTML(buf.String())
}
