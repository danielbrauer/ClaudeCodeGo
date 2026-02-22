package tui

import (
	"strings"
	"testing"
)

func TestParseShortstat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantNil  bool
		files    int
		added    int
		removed  int
	}{
		{
			name:    "full stat",
			input:   " 3 files changed, 10 insertions(+), 5 deletions(-)\n",
			files:   3,
			added:   10,
			removed: 5,
		},
		{
			name:    "only insertions",
			input:   " 1 file changed, 7 insertions(+)\n",
			files:   1,
			added:   7,
			removed: 0,
		},
		{
			name:    "only deletions",
			input:   " 2 files changed, 3 deletions(-)\n",
			files:   2,
			added:   0,
			removed: 3,
		},
		{
			name:    "empty",
			input:   "",
			wantNil: true,
		},
		{
			name:    "no match",
			input:   "nothing here",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseShortstat(tt.input)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil result")
			}
			if got.filesCount != tt.files {
				t.Errorf("filesCount = %d, want %d", got.filesCount, tt.files)
			}
			if got.linesAdded != tt.added {
				t.Errorf("linesAdded = %d, want %d", got.linesAdded, tt.added)
			}
			if got.linesRemoved != tt.removed {
				t.Errorf("linesRemoved = %d, want %d", got.linesRemoved, tt.removed)
			}
		})
	}
}

func TestParseNumstat(t *testing.T) {
	input := "10\t5\tsrc/main.go\n3\t0\tREADME.md\n-\t-\timage.png\n"

	stats, files := parseNumstat(input)

	if stats.filesCount != 3 {
		t.Errorf("filesCount = %d, want 3", stats.filesCount)
	}
	if stats.linesAdded != 13 {
		t.Errorf("linesAdded = %d, want 13", stats.linesAdded)
	}
	if stats.linesRemoved != 5 {
		t.Errorf("linesRemoved = %d, want 5", stats.linesRemoved)
	}
	if len(files) != 3 {
		t.Fatalf("len(files) = %d, want 3", len(files))
	}
	if files[0].path != "src/main.go" {
		t.Errorf("files[0].path = %q, want %q", files[0].path, "src/main.go")
	}
	if files[2].isBinary != true {
		t.Error("files[2].isBinary should be true for binary file")
	}
}

func TestParseNumstatEmpty(t *testing.T) {
	stats, files := parseNumstat("")
	if stats.filesCount != 0 {
		t.Errorf("filesCount = %d, want 0", stats.filesCount)
	}
	if len(files) != 0 {
		t.Errorf("len(files) = %d, want 0", len(files))
	}
}

func TestParseDiffOutput(t *testing.T) {
	input := `diff --git a/hello.go b/hello.go
index abc1234..def5678 100644
--- a/hello.go
+++ b/hello.go
@@ -1,5 +1,6 @@
 package main

+import "fmt"
+
 func main() {
-    println("hello")
+    fmt.Println("hello")
 }
`
	hunks := parseDiffOutput(input)

	if len(hunks) != 1 {
		t.Fatalf("expected 1 file in hunks, got %d", len(hunks))
	}

	h, ok := hunks["hello.go"]
	if !ok {
		t.Fatal("expected hunks for hello.go")
	}
	if len(h) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(h))
	}

	hunk := h[0]
	if hunk.oldStart != 1 || hunk.oldLines != 5 {
		t.Errorf("old range = %d,%d, want 1,5", hunk.oldStart, hunk.oldLines)
	}
	if hunk.newStart != 1 || hunk.newLines != 6 {
		t.Errorf("new range = %d,%d, want 1,6", hunk.newStart, hunk.newLines)
	}

	// Verify lines contain expected content.
	content := strings.Join(hunk.lines, "\n")
	if !strings.Contains(content, "+import \"fmt\"") {
		t.Error("expected hunk lines to contain '+import \"fmt\"'")
	}
	if !strings.Contains(content, "-    println(\"hello\")") {
		t.Error("expected hunk lines to contain '-    println(\"hello\")'")
	}
}

func TestParseDiffOutputMultipleFiles(t *testing.T) {
	input := `diff --git a/a.txt b/a.txt
--- a/a.txt
+++ b/a.txt
@@ -1,3 +1,3 @@
 line1
-line2
+line2-modified
 line3
diff --git a/b.txt b/b.txt
--- a/b.txt
+++ b/b.txt
@@ -1,2 +1,3 @@
 foo
+bar
 baz
`
	hunks := parseDiffOutput(input)

	if len(hunks) != 2 {
		t.Fatalf("expected 2 files in hunks, got %d", len(hunks))
	}

	if _, ok := hunks["a.txt"]; !ok {
		t.Error("expected hunks for a.txt")
	}
	if _, ok := hunks["b.txt"]; !ok {
		t.Error("expected hunks for b.txt")
	}
}

func TestParseDiffOutputEmpty(t *testing.T) {
	hunks := parseDiffOutput("")
	if len(hunks) != 0 {
		t.Errorf("expected 0 files, got %d", len(hunks))
	}
}

func TestPlural(t *testing.T) {
	if plural(1) != "" {
		t.Error("plural(1) should be empty")
	}
	if plural(0) != "s" {
		t.Error("plural(0) should be 's'")
	}
	if plural(2) != "s" {
		t.Error("plural(2) should be 's'")
	}
}

func TestClamp(t *testing.T) {
	if clamp(5, 1, 10) != 5 {
		t.Error("clamp(5,1,10) should be 5")
	}
	if clamp(0, 1, 10) != 1 {
		t.Error("clamp(0,1,10) should be 1")
	}
	if clamp(15, 1, 10) != 10 {
		t.Error("clamp(15,1,10) should be 10")
	}
}

func TestBuildReviewPrompt(t *testing.T) {
	prompt := buildReviewPrompt("42")
	if !strings.Contains(prompt, "PR number: 42") {
		t.Error("expected review prompt to contain PR number")
	}
	if !strings.Contains(prompt, "gh pr diff") {
		t.Error("expected review prompt to contain gh pr diff instruction")
	}

	emptyPrompt := buildReviewPrompt("")
	if !strings.Contains(emptyPrompt, "PR number: ") {
		t.Error("expected review prompt to contain PR number field")
	}
}

func TestRenderDiffViewCleanTree(t *testing.T) {
	d := &diffData{
		stats: diffStats{},
		files: nil,
		hunks: make(map[string][]diffHunk),
	}
	output := renderDiffView(d, 0, "list", 80)
	if !strings.Contains(output, "Working tree is clean") {
		t.Error("expected 'Working tree is clean' for empty diff")
	}
}

func TestRenderDiffViewWithFiles(t *testing.T) {
	d := &diffData{
		stats: diffStats{filesCount: 1, linesAdded: 5, linesRemoved: 2},
		files: []diffFile{
			{path: "main.go", linesAdded: 5, linesRemoved: 2},
		},
		hunks: map[string][]diffHunk{
			"main.go": {
				{oldStart: 1, oldLines: 3, newStart: 1, newLines: 6, lines: []string{
					" package main",
					"+import \"fmt\"",
					"-old line",
				}},
			},
		},
	}

	// List view.
	listOutput := renderDiffView(d, 0, "list", 80)
	if !strings.Contains(listOutput, "main.go") {
		t.Error("expected list view to contain file name")
	}
	if !strings.Contains(listOutput, "1 file changed") {
		t.Error("expected list view to contain stats")
	}

	// Detail view.
	detailOutput := renderDiffView(d, 0, "detail", 80)
	if !strings.Contains(detailOutput, "main.go") {
		t.Error("expected detail view to contain file name")
	}
	if !strings.Contains(detailOutput, "@@") {
		t.Error("expected detail view to contain hunk header")
	}
}
