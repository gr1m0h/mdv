package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func renderSample(t *testing.T) *Result {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "sample.md"))
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	res, err := New().Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return res
}

func render(t *testing.T, src string) *Result {
	t.Helper()
	res, err := New().Render([]byte(src))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return res
}

// R-1: all h1..h4 carry an id attribute.
func TestHeadingsHaveIDs(t *testing.T) {
	res := render(t, "# One\n\n## Two\n\n### Three\n\n#### Four\n")
	if len(res.TOC) != 4 {
		t.Fatalf("want 4 TOC entries, got %d", len(res.TOC))
	}
	for _, e := range res.TOC {
		if e.ID == "" {
			t.Errorf("heading %q has empty id", e.Text)
		}
		if !strings.Contains(res.HTML, `id="`+e.ID+`"`) {
			t.Errorf("html missing id %q", e.ID)
		}
	}
}

// R-2: Japanese heading id preserves non-ASCII.
func TestJapaneseHeadingID(t *testing.T) {
	res := render(t, "# mdv 動作確認\n")
	if got := res.TOC[0].ID; got != "mdv-動作確認" {
		t.Fatalf("want id %q, got %q", "mdv-動作確認", got)
	}
}

// R-3: duplicate headings get -1, -2 suffixes.
func TestDuplicateHeadingIDs(t *testing.T) {
	res := render(t, "# Dup\n\n# Dup\n\n# Dup\n")
	ids := []string{res.TOC[0].ID, res.TOC[1].ID, res.TOC[2].ID}
	want := []string{"dup", "dup-1", "dup-2"}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("id[%d] = %q, want %q", i, ids[i], want[i])
		}
	}
}

// R-4: GFM tables render as <table>.
func TestTable(t *testing.T) {
	res := render(t, "| a | b |\n|---|---|\n| 1 | 2 |\n")
	if !strings.Contains(res.HTML, "<table>") {
		t.Errorf("expected <table>, got: %s", res.HTML)
	}
}

// R-5: mermaid fence becomes <div class="mermaid"> with no language-mermaid.
func TestMermaid(t *testing.T) {
	res := render(t, "```mermaid\nflowchart LR\n A-->B\n```\n")
	if !strings.Contains(res.HTML, `<div class="mermaid">`) {
		t.Errorf("expected mermaid div, got: %s", res.HTML)
	}
	if strings.Contains(res.HTML, "language-mermaid") {
		t.Errorf("language-mermaid should not remain: %s", res.HTML)
	}
	if !res.HasMermaid {
		t.Errorf("HasMermaid should be true")
	}
}

// R-6: [!WARNING] gets .alert-warning and the marker is removed.
func TestAlertWarning(t *testing.T) {
	res := render(t, "> [!WARNING]\n> danger ahead\n")
	if !strings.Contains(res.HTML, "alert-warning") {
		t.Errorf("expected alert-warning: %s", res.HTML)
	}
	if strings.Contains(res.HTML, "[!WARNING]") {
		t.Errorf("marker should be removed: %s", res.HTML)
	}
}

// R-7: [!TIP] gets .alert-tip and a title row.
func TestAlertTip(t *testing.T) {
	res := render(t, "> [!TIP]\n> helpful\n")
	if !strings.Contains(res.HTML, "alert-tip") {
		t.Errorf("expected alert-tip: %s", res.HTML)
	}
	if !strings.Contains(res.HTML, "alert-title") {
		t.Errorf("expected alert-title: %s", res.HTML)
	}
}

// R-8/R-9/R-10: chroma token classes for go/hcl/dockerfile.
func TestHighlightLanguages(t *testing.T) {
	for _, lang := range []string{"go", "hcl", "dockerfile"} {
		src := "```" + lang + "\nfoo bar\n```\n"
		res := render(t, src)
		if !strings.Contains(res.HTML, `class="chroma"`) {
			t.Errorf("lang %s: expected chroma wrapper, got: %s", lang, res.HTML)
		}
	}
}

// R-11: unknown language falls back to plain text without error.
func TestUnknownLanguage(t *testing.T) {
	res := render(t, "```wat-unknown\nsome text\n```\n")
	if !strings.Contains(res.HTML, "some text") {
		t.Errorf("expected code text preserved: %s", res.HTML)
	}
}

// R-12: <script> is removed by sanitize.
func TestScriptSanitized(t *testing.T) {
	res := render(t, "hello\n\n<script>alert(1)</script>\n\nworld\n")
	if strings.Contains(res.HTML, "<script") {
		t.Errorf("script tag should be stripped: %s", res.HTML)
	}
}

// R-13: TOC entries reference ids that exist in the HTML.
func TestTOCReferencesExist(t *testing.T) {
	res := renderSample(t)
	if len(res.TOC) < 2 {
		t.Fatalf("expected several TOC entries, got %d", len(res.TOC))
	}
	for _, e := range res.TOC {
		if !strings.Contains(res.HTML, `id="`+e.ID+`"`) {
			t.Errorf("TOC id %q not found in HTML", e.ID)
		}
	}
}

// R-14: document title comes from the first h1.
func TestTitleFromH1(t *testing.T) {
	res := renderSample(t)
	if res.Title != "mdv 動作確認" {
		t.Errorf("want title %q, got %q", "mdv 動作確認", res.Title)
	}
}

// Task lists survive sanitization.
func TestTaskList(t *testing.T) {
	res := render(t, "- [x] done\n- [ ] todo\n")
	if !strings.Contains(res.HTML, `type="checkbox"`) {
		t.Errorf("expected checkbox input: %s", res.HTML)
	}
}

// Footnotes keep their anchor ids/hrefs (U-4).
func TestFootnote(t *testing.T) {
	res := render(t, "text[^1]\n\n[^1]: note\n")
	if !strings.Contains(res.HTML, "fn:1") && !strings.Contains(res.HTML, "fnref") {
		t.Errorf("expected footnote anchors: %s", res.HTML)
	}
}
