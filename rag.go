// Package rag implements a lightweight in-memory Retrieval-Augmented Generation
// index over Zatrano source files. It uses TF-IDF scoring so the agent can
// retrieve only the most relevant files for a given query, staying well within
// the context window of a 7B model running on 8GB RAM.
//
// No external dependencies — pure Go, no vector DB needed.
package rag

import (
	"math"
	"path/filepath"
	"sort"
	"strings"
	"zatrano-agent/internal/loader"
)

// Document is an indexed source file.
type Document struct {
	Path    string
	Content string
	TF      map[string]float64 // term frequency
}

// Index holds all indexed documents and the global IDF table.
type Index struct {
	docs []Document
	idf  map[string]float64
}

// Build creates a new RAG index from the loaded project context.
func Build(ctx *loader.ProjectContext) *Index {
	idx := &Index{
		docs: make([]Document, 0, len(ctx.Files)),
		idf:  map[string]float64{},
	}

	// Build term frequencies per document
	df := map[string]int{} // document frequency
	for _, f := range ctx.Files {
		terms := tokenize(f.Content + " " + f.RelPath)
		tf := computeTF(terms)
		idx.docs = append(idx.docs, Document{
			Path:    f.RelPath,
			Content: f.Content,
			TF:      tf,
		})
		for t := range tf {
			df[t]++
		}
	}

	// Compute IDF
	N := float64(len(idx.docs))
	for term, count := range df {
		idx.idf[term] = math.Log(N/(float64(count)+1)) + 1
	}

	return idx
}

// Search returns the top-k most relevant documents for the query.
func (idx *Index) Search(query string, topK int) []Document {
	if len(idx.docs) == 0 {
		return nil
	}

	queryTerms := tokenize(query)
	type scored struct {
		doc   Document
		score float64
	}

	results := make([]scored, 0, len(idx.docs))
	for _, doc := range idx.docs {
		score := cosineSim(queryTerms, doc.TF, idx.idf)
		// Boost files whose path contains query terms
		pathLower := strings.ToLower(doc.Path)
		for _, t := range queryTerms {
			if strings.Contains(pathLower, t) {
				score += 0.3
			}
		}
		results = append(results, scored{doc, score})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if topK > len(results) {
		topK = len(results)
	}
	out := make([]Document, topK)
	for i := range out {
		out[i] = results[i].doc
	}
	return out
}

// BuildContext returns a compact context string from the top-k results.
// maxChars limits the total character count to keep prompts manageable.
func (idx *Index) BuildContext(query string, topK int, maxChars int) string {
	docs := idx.Search(query, topK)
	var sb strings.Builder
	sb.WriteString("=== ZATRANO İLGİLİ DOSYALAR ===\n\n")
	total := 0
	for _, doc := range docs {
		chunk := formatDoc(doc)
		if total+len(chunk) > maxChars {
			// Include a truncated version
			remaining := maxChars - total
			if remaining > 200 {
				sb.WriteString(chunk[:remaining])
				sb.WriteString("\n... (dosya kısaltıldı)\n\n")
			}
			break
		}
		sb.WriteString(chunk)
		total += len(chunk)
	}
	return sb.String()
}

// Size returns the number of indexed documents.
func (idx *Index) Size() int { return len(idx.docs) }

// ── Helpers ───────────────────────────────────────────────────────────────────

func formatDoc(doc Document) string {
	ext := strings.ToLower(filepath.Ext(doc.Path))
	lang := "go"
	if ext == ".md" {
		lang = "markdown"
	}
	return "--- FILE: " + doc.Path + " ---\n" +
		"```" + lang + "\n" +
		doc.Content + "\n" +
		"```\n\n"
}

func tokenize(text string) []string {
	// Lowercase, split on non-alphanumeric, filter short tokens
	text = strings.ToLower(text)
	var tokens []string
	current := strings.Builder{}

	for _, r := range text {
		if isAlphaNum(r) {
			current.WriteRune(r)
		} else {
			if current.Len() >= 3 {
				tokens = append(tokens, current.String())
			}
			current.Reset()
		}
	}
	if current.Len() >= 3 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

func isAlphaNum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_'
}

func computeTF(terms []string) map[string]float64 {
	counts := map[string]int{}
	for _, t := range terms {
		counts[t]++
	}
	tf := map[string]float64{}
	total := float64(len(terms))
	if total == 0 {
		return tf
	}
	for t, c := range counts {
		tf[t] = float64(c) / total
	}
	return tf
}

func cosineSim(queryTerms []string, docTF map[string]float64, idf map[string]float64) float64 {
	score := 0.0
	for _, t := range queryTerms {
		tfidf := docTF[t] * idf[t]
		score += tfidf
	}
	return score
}
