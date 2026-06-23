package notify

import (
	"bytes"
	_ "embed"
	"html/template"
)

//go:embed templates/layout.html
var layoutHTML string

var layoutTmpl = template.Must(template.New("layout").Parse(layoutHTML))

type layoutData struct {
	Content template.HTML
}

// renderLayout wraps the rendered content HTML in the branded email layout.
func renderLayout(content string) (string, error) {
	var buf bytes.Buffer
	err := layoutTmpl.Execute(&buf, layoutData{
		Content: template.HTML(content), //nolint:gosec // content is already rendered by html/template with data escaping
	})
	return buf.String(), err
}
