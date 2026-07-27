package placeholders

import "testing"

// Spec §2: {{name}} / {{name=default}}, name = [A-Za-z0-9_-]+, no escapes,
// repeats prompt once (first default wins), verbatim substitution.

func TestParseFindsNamedPlaceholders(t *testing.T) {
	got := Parse("ssh {{host=prod-1}} 'deploy {{env}}'")
	want := []Placeholder{
		{Name: "host", Default: "prod-1", HasDefault: true},
		{Name: "env"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseLeavesOtherBraceFormsLiteral(t *testing.T) {
	if got := Parse("awk '{print $1}' ${HOME}/f {{ spaced }} {1}"); len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}

func TestParseRepeatedNameAppearsOnce(t *testing.T) {
	got := Parse("cp {{f=a.txt}} {{f=b.txt}}.bak {{f}}")
	if len(got) != 1 || got[0].Name != "f" || got[0].Default != "a.txt" {
		t.Errorf("got %+v, want one placeholder f with default a.txt", got)
	}
}

func TestRender(t *testing.T) {
	cases := []struct {
		label    string
		template string
		values   map[string]string
		want     string
	}{
		{"verbatim substitution", `git commit -m "{{msg}}"`, map[string]string{"msg": `fix: a "quoted" thing`}, `git commit -m "fix: a "quoted" thing"`},
		{"repeated name fills every occurrence", "cp {{f}} {{f}}.bak", map[string]string{"f": "notes.md"}, "cp notes.md notes.md.bak"},
		{"empty value renders empty", "ls {{flags}} .", map[string]string{"flags": ""}, "ls  ."},
		{"missing falls back to default then empty", "ssh {{host=prod-1}} {{cmd}}", map[string]string{}, "ssh prod-1 "},
		{"first default wins everywhere", "cp {{f=a.txt}} {{f=b.txt}}.bak", map[string]string{}, "cp a.txt a.txt.bak"},
		{"literal brace forms pass through", "awk '{print $1}' ${HOME}", map[string]string{}, "awk '{print $1}' ${HOME}"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			if got := Render(tc.template, tc.values); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRenderSegmentsFlagsSubstitutions(t *testing.T) {
	segs := RenderSegments("ssh {{host=prod-1}} x", map[string]string{})
	if len(segs) != 3 {
		t.Fatalf("got %d segments, want 3: %+v", len(segs), segs)
	}
	if segs[0].Text != "ssh " || segs[0].Flag {
		t.Errorf("segment 0 = %+v", segs[0])
	}
	if segs[1].Text != "prod-1" || !segs[1].Flag {
		t.Errorf("segment 1 = %+v", segs[1])
	}
}

func TestTemplateSegmentsKeepsRawTokens(t *testing.T) {
	segs := TemplateSegments("ssh {{host=prod-1}}")
	if segs[1].Text != "{{host=prod-1}}" || !segs[1].Flag {
		t.Errorf("segment 1 = %+v, want the raw token flagged", segs[1])
	}
}
