// Package writer parses LLM responses for file paths and code blocks,
// then writes them to the Zatrano project directory.
package writer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ParsedFile represents a single file extracted from an LLM response.
type ParsedFile struct {
	Path    string // relative path, e.g. "models/order.go"
	Content string // file content
	Lang    string // language hint from code fence
}

// WriteResult is the result of writing one file.
type WriteResult struct {
	Path    string
	Written bool
	Created bool // true if new file, false if overwritten
	Error   error
}

// pathCommentRe matches lines like:
//
//	// models/order.go
//	// FILE: models/order.go
//	// Path: models/order.go
var pathCommentRe = regexp.MustCompile(
	`(?i)^(?://|#)\s*(?:file[:\s]*|path[:\s]*)?([a-zA-Z0-9_./-]+\.go)\s*$`,
)

// codeFenceRe matches ``` lang \n content ```.
var codeFenceRe = regexp.MustCompile("(?s)```(\\w*)\\n(.*?)```")

// Parse scans an LLM response and extracts all (path, code) pairs.
// It handles two patterns:
//  1. A path comment immediately before a code fence:
//     // models/order.go
//     ```go
//     package models ...
//     ```
//  2. A path comment inside the code fence as the first line:
//     ```go
//     // models/order.go
//     package models ...
//     ```
func Parse(response string) []ParsedFile {
	var files []ParsedFile
	seen := map[string]bool{}

	matches := codeFenceRe.FindAllStringSubmatchIndex(response, -1)
	for _, m := range matches {
		lang := response[m[2]:m[3]]
		code := strings.TrimSpace(response[m[4]:m[5]])

		// Strategy 1: look for path comment on the line before the fence
		path := extractPathBefore(response, m[0])

		// Strategy 2: first line inside the fence
		if path == "" {
			lines := strings.SplitN(code, "\n", 2)
			if len(lines) > 0 {
				if p := matchPathComment(strings.TrimSpace(lines[0])); p != "" {
					path = p
					if len(lines) > 1 {
						code = strings.TrimSpace(lines[1])
					}
				}
			}
		}

		if path == "" || seen[path] {
			continue
		}
		if lang == "" {
			lang = "go"
		}
		seen[path] = true
		files = append(files, ParsedFile{
			Path:    path,
			Content: code,
			Lang:    lang,
		})
	}
	return files
}

// extractPathBefore looks at the text immediately before fenceStart for a path comment.
func extractPathBefore(text string, fenceStart int) string {
	before := text[:fenceStart]
	// Get the last non-empty line before the fence
	lines := strings.Split(strings.TrimRight(before, " \t\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if p := matchPathComment(line); p != "" {
			return p
		}
		break // stop if the previous non-empty line isn't a path
	}
	return ""
}

func matchPathComment(line string) string {
	m := pathCommentRe.FindStringSubmatch(line)
	if m != nil {
		return m[1]
	}
	return ""
}

// WriteAll writes all parsed files to projectDir.
// It creates directories as needed and never writes outside projectDir.
func WriteAll(projectDir string, files []ParsedFile) []WriteResult {
	results := make([]WriteResult, 0, len(files))
	for _, f := range files {
		results = append(results, Write(projectDir, f))
	}
	return results
}

// Write writes a single ParsedFile to disk.
func Write(projectDir string, f ParsedFile) WriteResult {
	res := WriteResult{Path: f.Path}

	// Security: reject absolute paths or path traversal
	if filepath.IsAbs(f.Path) || strings.Contains(f.Path, "..") {
		res.Error = fmt.Errorf("güvenli olmayan yol: %s", f.Path)
		return res
	}

	fullPath := filepath.Join(projectDir, filepath.FromSlash(f.Path))

	// Check if it's an existing file (for Created flag)
	_, err := os.Stat(fullPath)
	res.Created = os.IsNotExist(err)

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		res.Error = fmt.Errorf("dizin oluşturulamadı: %w", err)
		return res
	}

	// Write file
	if err := os.WriteFile(fullPath, []byte(f.Content+"\n"), 0644); err != nil {
		res.Error = fmt.Errorf("dosya yazılamadı: %w", err)
		return res
	}

	res.Written = true
	return res
}

// Summary returns a human-readable summary of write results.
func Summary(results []WriteResult) string {
	var sb strings.Builder
	created, updated, failed := 0, 0, 0
	for _, r := range results {
		if r.Error != nil {
			failed++
			sb.WriteString(fmt.Sprintf("  ❌ %s: %v\n", r.Path, r.Error))
		} else if r.Created {
			created++
			sb.WriteString(fmt.Sprintf("  ✅ %s (yeni)\n", r.Path))
		} else {
			updated++
			sb.WriteString(fmt.Sprintf("  📝 %s (güncellendi)\n", r.Path))
		}
	}
	sb.WriteString(fmt.Sprintf("\nToplam: %d yeni, %d güncellendi, %d hata", created, updated, failed))
	return sb.String()
}
