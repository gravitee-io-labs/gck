package notes

import (
	"bytes"
	"text/template"
)

// RenderWithFlags executes templateContent as a Go text/template with data as
// the dot context, registering a hasFlag function that returns true when the
// named context flag was activated on the CLI. This lets a notes file gate an
// endpoint row or a paragraph on the flags the user passed
// (e.g. {{ not (hasFlag "disable-portal") }}).
func RenderWithFlags(templateContent string, data any, activeFlags []string) (string, error) {
	flagSet := make(map[string]bool, len(activeFlags))
	for _, f := range activeFlags {
		flagSet[f] = true
	}
	funcMap := template.FuncMap{
		"hasFlag": func(name string) bool { return flagSet[name] },
	}
	tmpl, err := template.New("notes").Funcs(funcMap).Parse(templateContent)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
