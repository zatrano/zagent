// Package diff computes and formats unified diffs between existing files
// and LLM-generated content.
package diff

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"zatrano-agent/internal/writer"
)

// FileDiff holds the diff result for a single file.
type FileDiff struct {
	Path     string
	OldLines []string
	NewLines []string
	Hunks    []Hunk
	IsNew    bool // file does not exist yet
}

// Hunk represents a changed region.
type Hunk struct {
	OldStart int
	NewStart int
	Lines    []DiffLine
}

// DiffLine is one line in a hunk, with its change type.
type DiffLine struct {
	Kind    LineKind
	Content string
}

// LineKind indicates whether a line is context, addition, or deletion.
type LineKind int

const (
	LineContext LineKind = iota
	LineAdd
	LineDelete
)

// Compute generates diffs for all parsed files against the project on disk.
func Compute(projectDir string, files []writer.ParsedFile) []FileDiff {
	diffs := make([]FileDiff, 0, len(files))
	for _, f := range files {
		diffs = append(diffs, computeOne(projectDir, f))
	}
	return diffs
}

func computeOne(projectDir string, f writer.ParsedFile) FileDiff {
	fullPath := filepath.Join(projectDir, filepath.FromSlash(f.Path))

	existingBytes, err := os.ReadFile(fullPath)
	isNew := os.IsNotExist(err)

	oldLines := []string{}
	if !isNew && err == nil {
		oldLines = splitLines(string(existingBytes))
	}
	newLines := splitLines(f.Content)

	hunks := computeHunks(oldLines, newLines)

	return FileDiff{
		Path:     f.Path,
		OldLines: oldLines,
		NewLines: newLines,
		Hunks:    hunks,
		IsNew:    isNew,
	}
}

func splitLines(s string) []string {
	lines := strings.Split(s, "\n")
	// Remove trailing empty line from a trailing newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// computeHunks uses a simple LCS-based diff algorithm.
func computeHunks(old, new []string) []Hunk {
	edits := lcs(old, new)
	if len(edits) == 0 {
		return nil
	}

	const context = 3
	var hunks []Hunk
	var current *Hunk

	for _, e := range edits {
		if current == nil || e.OldIdx > current.OldStart+len(current.Lines)+context {
			if current != nil {
				hunks = append(hunks, *current)
			}
			start := e.OldIdx - context
			if start < 0 {
				start = 0
			}
			current = &Hunk{OldStart: start, NewStart: e.NewIdx - context}
			if current.NewStart < 0 {
				current.NewStart = 0
			}
		}
		current.Lines = append(current.Lines, DiffLine{Kind: e.Kind, Content: e.Content})
	}
	if current != nil {
		hunks = append(hunks, *current)
	}
	return hunks
}

type edit struct {
	Kind    LineKind
	OldIdx  int
	NewIdx  int
	Content string
}

// lcs computes the edit script using Myers diff (simplified O(ND)).
func lcs(old, new []string) []edit {
	type point struct{ x, y int }
	n, m := len(old), len(new)
	if n == 0 && m == 0 {
		return nil
	}

	// Build edit list via simple forward diff
	var edits []edit
	i, j := 0, 0
	for i < n || j < m {
		if i < n && j < m && old[i] == new[j] {
			i++
			j++
			continue
		}
		// Greedy: prefer delete then add
		if i < n && (j >= m || !lineInSlice(old[i], new[j:])) {
			edits = append(edits, edit{Kind: LineDelete, OldIdx: i, NewIdx: j, Content: old[i]})
			i++
		} else if j < m {
			edits = append(edits, edit{Kind: LineAdd, OldIdx: i, NewIdx: j, Content: new[j]})
			j++
		}
	}
	return edits
}

func lineInSlice(s string, slice []string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// FormatUnified returns a unified diff string (similar to `diff -u`).
func FormatUnified(d FileDiff) string {
	if len(d.Hunks) == 0 && !d.IsNew {
		return fmt.Sprintf("--- %s\n+++ %s\n(değişiklik yok)\n", d.Path, d.Path)
	}

	var sb strings.Builder
	if d.IsNew {
		sb.WriteString(fmt.Sprintf("--- /dev/null\n+++ %s (yeni dosya)\n", d.Path))
		sb.WriteString("@@ -0,0 +1," + fmt.Sprint(len(d.NewLines)) + " @@\n")
		for _, l := range d.NewLines {
			sb.WriteString("+" + l + "\n")
		}
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("--- %s\n+++ %s\n", d.Path, d.Path))
	for _, h := range d.Hunks {
		sb.WriteString(fmt.Sprintf("@@ -%d +%d @@\n", h.OldStart+1, h.NewStart+1))
		for _, l := range h.Lines {
			switch l.Kind {
			case LineAdd:
				sb.WriteString("+" + l.Content + "\n")
			case LineDelete:
				sb.WriteString("-" + l.Content + "\n")
			default:
				sb.WriteString(" " + l.Content + "\n")
			}
		}
	}
	return sb.String()
}

// HTMLDiff returns a side-by-side HTML diff for web display.
func HTMLDiff(d FileDiff) string {
	var sb strings.Builder
	sb.WriteString(`<div class="diff-wrap">`)
	sb.WriteString(fmt.Sprintf(`<div class="diff-header"><span class="diff-path">%s</span>`, escHTML(d.Path)))
	if d.IsNew {
		sb.WriteString(`<span class="diff-badge new">YENİ DOSYA</span>`)
	} else if len(d.Hunks) == 0 {
		sb.WriteString(`<span class="diff-badge same">DEĞİŞİKLİK YOK</span>`)
	} else {
		adds := countKind(d.Hunks, LineAdd)
		dels := countKind(d.Hunks, LineDelete)
		sb.WriteString(fmt.Sprintf(`<span class="diff-badge add">+%d</span><span class="diff-badge del">-%d</span>`, adds, dels))
	}
	sb.WriteString(`</div>`)
	sb.WriteString(`<div class="diff-body">`)

	if d.IsNew {
		for i, l := range d.NewLines {
			sb.WriteString(fmt.Sprintf(
				`<div class="diff-line add"><span class="diff-ln">%d</span><span class="diff-sign">+</span><span class="diff-code">%s</span></div>`,
				i+1, escHTML(l),
			))
		}
	} else {
		for _, h := range d.Hunks {
			oldN, newN := h.OldStart+1, h.NewStart+1
			for _, l := range h.Lines {
				cls := "ctx"
				sign := " "
				switch l.Kind {
				case LineAdd:
					cls = "add"
					sign = "+"
				case LineDelete:
					cls = "del"
					sign = "-"
				}
				sb.WriteString(fmt.Sprintf(
					`<div class="diff-line %s"><span class="diff-ln">%d/%d</span><span class="diff-sign">%s</span><span class="diff-code">%s</span></div>`,
					cls, oldN, newN, sign, escHTML(l.Content),
				))
				if l.Kind != LineAdd {
					oldN++
				}
				if l.Kind != LineDelete {
					newN++
				}
			}
		}
	}

	sb.WriteString(`</div></div>`)
	return sb.String()
}

func countKind(hunks []Hunk, k LineKind) int {
	n := 0
	for _, h := range hunks {
		for _, l := range h.Lines {
			if l.Kind == k {
				n++
			}
		}
	}
	return n
}

func escHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\t", "    ")
	return s
}
