// Package placeholders handles {{name}} / {{name=default}} (spec §2).
// Anything else stays literal; substitution is verbatim — template authors do
// their own quoting.
package placeholders

import "regexp"

type Placeholder struct {
	Name       string
	Default    string
	HasDefault bool
}

var re = regexp.MustCompile(`\{\{([A-Za-z0-9_-]+)(?:=([^}]*))?\}\}`)

// Parse returns the Placeholders unique by name (a repeat prompts once);
// the first default wins.
func Parse(template string) []Placeholder {
	seen := map[string]bool{}
	out := []Placeholder{}
	for _, m := range re.FindAllStringSubmatchIndex(template, -1) {
		name := template[m[2]:m[3]]
		if seen[name] {
			continue
		}
		seen[name] = true
		p := Placeholder{Name: name}
		if m[4] >= 0 {
			p.Default = template[m[4]:m[5]]
			p.HasDefault = true
		}
		out = append(out, p)
	}
	return out
}

// resolve gives every name its value: supplied > first-wins default > empty,
// so every occurrence of a repeated name renders identically.
func resolve(template string, values map[string]string) map[string]string {
	resolved := map[string]string{}
	for _, p := range Parse(template) {
		if value, ok := values[p.Name]; ok {
			resolved[p.Name] = value
		} else {
			resolved[p.Name] = p.Default
		}
	}
	return resolved
}

func Render(template string, values map[string]string) string {
	resolved := resolve(template, values)
	out := ""
	last := 0
	for _, m := range re.FindAllStringSubmatchIndex(template, -1) {
		out += template[last:m[0]] + resolved[template[m[2]:m[3]]]
		last = m[1]
	}
	return out + template[last:]
}

// Segment is a run of the template, flagged as either literal or the thing
// the caller wants highlighted.
type Segment struct {
	Text string
	Flag bool
}

// TemplateSegments splits into literal/placeholder runs with the raw {{...}}
// tokens kept, so the edit screen can highlight the Placeholder slots.
func TemplateSegments(template string) []Segment {
	out := []Segment{}
	last := 0
	for _, m := range re.FindAllStringSubmatchIndex(template, -1) {
		if m[0] > last {
			out = append(out, Segment{Text: template[last:m[0]]})
		}
		out = append(out, Segment{Text: template[m[0]:m[1]], Flag: true})
		last = m[1]
	}
	if last < len(template) {
		out = append(out, Segment{Text: template[last:]})
	}
	return out
}

// RenderSegments splits into literal/substituted runs so the live preview can
// highlight the substituted values (spec §3.2).
func RenderSegments(template string, values map[string]string) []Segment {
	resolved := resolve(template, values)
	out := []Segment{}
	last := 0
	for _, m := range re.FindAllStringSubmatchIndex(template, -1) {
		if m[0] > last {
			out = append(out, Segment{Text: template[last:m[0]]})
		}
		out = append(out, Segment{Text: resolved[template[m[2]:m[3]]], Flag: true})
		last = m[1]
	}
	if last < len(template) {
		out = append(out, Segment{Text: template[last:]})
	}
	return out
}
