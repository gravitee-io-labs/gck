package notes

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/fatih/color"
	"gopkg.in/yaml.v3"
)

// Layer is one context's notes file, carried through composition unrendered.
// Rendering is deferred to print time because both the `when` guards and the
// prose body may depend on the context flags the user activated on the CLI,
// which are only known once the command runs.
type Layer struct {
	Source string `yaml:"source"` // registry context path the note came from
	Raw    string `yaml:"raw"`
}

// Condition is a `when` guard: a template expression that must render truthy
// for its entry to appear. It unmarshals from any YAML scalar, so both
// `when: false` and `when: '{{ not (hasFlag "disable-ui") }}'` are valid.
type Condition string

func (c *Condition) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("line %d: when must be a scalar", n.Line)
	}
	*c = Condition(n.Value)
	return nil
}

// Endpoint is one row of the merged endpoints table. Name is the merge key,
// so it must identify the service across every context it can be composed
// with -- "APIM API" rather than "Management API".
type Endpoint struct {
	Name string     `yaml:"name"`
	URL  string     `yaml:"url"`
	Note string     `yaml:"note,omitempty"`
	When *Condition `yaml:"when,omitempty"`

	// Origin is the context path that supplied this row. It is derived during
	// the merge, never authored, and lets the registry site attribute an
	// inherited row to the context that declared it.
	Origin string `yaml:"-"`
}

// document is a layer's front matter plus its prose body.
type document struct {
	Title     string     `yaml:"title,omitempty"`
	Endpoints []Endpoint `yaml:"endpoints,omitempty"`
	When      *Condition `yaml:"when,omitempty"`

	body string
}

// Block is a titled prose section contributed by one layer.
type Block struct {
	Title string
	Body  string
}

// Merged is every layer folded together: one endpoints table and the prose
// blocks behind it. Endpoints keeps rows whose `when` guard is false, so
// callers checking a context against its config can see what it documents
// rather than only what is on screen right now.
type Merged struct {
	Endpoints []Endpoint
	Blocks    []Block
}

// VisibleEndpoints returns the rows whose `when` guard passes.
func (m Merged) VisibleEndpoints() []Endpoint {
	visible := make([]Endpoint, 0, len(m.Endpoints))
	for _, ep := range m.Endpoints {
		if truthy(ep.When) {
			visible = append(visible, ep)
		}
	}
	return visible
}

const frontMatterDelim = "---"

// Append appends src to base, skipping layers whose Source is already there.
// A context can be reached twice in one composition -- e.g. "--from
// grafana/base --from otel-collector/base", where grafana/base already
// composes the collector -- and its notes should still be printed once.
func Append(base, src []Layer) []Layer {
	if len(src) == 0 {
		return base
	}
	seen := make(map[string]bool, len(base))
	for _, l := range base {
		seen[l.Source] = true
	}
	out := make([]Layer, 0, len(base)+len(src))
	out = append(out, base...)
	for _, l := range src {
		if seen[l.Source] {
			continue
		}
		seen[l.Source] = true
		out = append(out, l)
	}
	return out
}

// Compose renders every layer with data and activeFlags, folds them into a
// single document and formats it for the terminal. Layers contribute rows to
// one shared endpoints table; their prose bodies follow, each under its own
// title, in composition order.
func Compose(layers []Layer, data any, activeFlags []string) (string, error) {
	merged, err := Merge(layers, data, activeFlags)
	if err != nil {
		return "", err
	}

	return format(merged.VisibleEndpoints(), merged.Blocks), nil
}

// Merge renders every layer and folds them into one document, without
// formatting. Rows and blocks merge by Name and Title respectively: the first
// layer to declare one fixes its position, the last one to declare it supplies
// the content.
func Merge(layers []Layer, data any, activeFlags []string) (Merged, error) {
	var (
		endpoints []Endpoint
		byName    = make(map[string]int)
		blocks    []Block
		byTitle   = make(map[string]int)
	)

	for _, l := range layers {
		doc, err := parseLayer(l, data, activeFlags)
		if err != nil {
			return Merged{}, err
		}
		if doc == nil {
			continue
		}
		for _, ep := range doc.Endpoints {
			if ep.Name == "" {
				return Merged{}, fmt.Errorf("notes from %s: endpoint is missing a name", l.Source)
			}
			// Keep the position of the first layer that declared the row so
			// the table reads in composition order, but let a later layer
			// replace it -- correcting the URL, or gating it off with
			// `when: false` when that layer stops exposing the service.
			ep.Origin = l.Source
			if i, ok := byName[ep.Name]; ok {
				endpoints[i] = ep
				continue
			}
			byName[ep.Name] = len(endpoints)
			endpoints = append(endpoints, ep)
		}
		body := trimBlankLines(doc.body)
		if body == "" {
			continue
		}
		// Title is the merge key for prose, as Name is for endpoints: a later
		// layer that changes how a component works restates it under the same
		// title and replaces the inherited instructions in place.
		if doc.Title != "" {
			if i, ok := byTitle[doc.Title]; ok {
				blocks[i].Body = body
				continue
			}
			byTitle[doc.Title] = len(blocks)
		}
		blocks = append(blocks, Block{Title: doc.Title, Body: body})
	}

	return Merged{Endpoints: endpoints, Blocks: blocks}, nil
}

// parseLayer renders a layer as a template, splits front matter from body and
// unmarshals the front matter. It returns nil when the layer's own `when`
// gate is false.
func parseLayer(l Layer, data any, activeFlags []string) (*document, error) {
	rendered, err := RenderWithFlags(l.Raw, data, activeFlags)
	if err != nil {
		return nil, fmt.Errorf("rendering notes from %s: %w", l.Source, err)
	}

	front, body := splitFrontMatter(rendered)

	var doc document
	if front != "" {
		if err := yaml.Unmarshal([]byte(front), &doc); err != nil {
			return nil, fmt.Errorf("parsing notes front matter from %s: %w", l.Source, err)
		}
	}
	if !truthy(doc.When) {
		return nil, nil
	}
	doc.body = body
	return &doc, nil
}

// splitFrontMatter separates a leading "---"-delimited YAML block from the
// prose that follows. A file with no leading delimiter is all prose.
func splitFrontMatter(s string) (front, body string) {
	lines := strings.Split(s, "\n")

	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	if start == len(lines) || strings.TrimSpace(lines[start]) != frontMatterDelim {
		return "", s
	}
	for i := start + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == frontMatterDelim {
			return strings.Join(lines[start+1:i], "\n"), strings.Join(lines[i+1:], "\n")
		}
	}
	return "", s
}

// truthy reports whether a rendered `when` expression should show its entry.
// Go templates render booleans as "true"/"false", so `{{ not (hasFlag "x") }}`
// works as written. A nil pointer means the field was absent.
func truthy(when *Condition) bool {
	if when == nil {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(string(*when))) {
	case "", "false", "0", "no", "off", "<no value>":
		return false
	}
	return true
}

// format lays out the merged endpoints table and the prose blocks. Everything
// is indented to sit under the command output that precedes it.
func format(endpoints []Endpoint, blocks []Block) string {
	var b strings.Builder

	if len(endpoints) > 0 {
		b.WriteString(heading("Endpoints"))
		b.WriteString("\n\n")
		for _, line := range endpointLines(endpoints) {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	for _, blk := range blocks {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		if blk.Title != "" {
			b.WriteString(heading(blk.Title))
			b.WriteString("\n\n")
		}
		b.WriteString(indent(blk.Body))
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

func heading(title string) string {
	return "  " + color.BlueString(title)
}

// endpointLines renders the table with name, URL and note columns aligned.
func endpointLines(endpoints []Endpoint) []string {
	nameW, urlW := 0, 0
	for _, ep := range endpoints {
		if n := utf8.RuneCountInString(ep.Name); n > nameW {
			nameW = n
		}
		if ep.Note != "" {
			if n := utf8.RuneCountInString(ep.URL); n > urlW {
				urlW = n
			}
		}
	}

	lines := make([]string, 0, len(endpoints))
	for _, ep := range endpoints {
		line := "    " + pad(ep.Name, nameW) + "  " + ep.URL
		if ep.Note != "" {
			line = "    " + pad(ep.Name, nameW) + "  " + pad(ep.URL, urlW) + "  " + ep.Note
		}
		lines = append(lines, strings.TrimRight(line, " "))
	}
	return lines
}

func pad(s string, width int) string {
	if n := width - utf8.RuneCountInString(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// trimBlankLines drops blank lines from both ends of a prose body while
// preserving the indentation of the lines that remain -- TrimSpace would eat
// the leading indent of a body that opens on an indented command.
func trimBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	if start == end {
		return ""
	}
	return strings.Join(lines[start:end], "\n")
}

// indent shifts a prose body under its heading, leaving blank lines empty.
func indent(body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
			continue
		}
		lines[i] = strings.TrimRight("    "+line, " ")
	}
	return strings.Join(lines, "\n")
}
