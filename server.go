// Package web provides an HTTP server exposing the agent via REST + SSE.
package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
	"zatrano-agent/internal/agent"
	"zatrano-agent/internal/diff"
	"zatrano-agent/internal/llm"
	"zatrano-agent/internal/writer"
)

// Server wraps the agent and serves an HTTP interface.
type Server struct {
	ag         *agent.Agent
	llmCfg     llm.Config
	projectDir string
	port       int
}

// NewServer creates a new web server.
func NewServer(cfg llm.Config, projectDir string, port int) *Server {
	return &Server{
		ag:         agent.New(cfg, projectDir),
		llmCfg:     cfg,
		projectDir: projectDir,
		port:       port,
	}
}

// LoadProject loads the Zatrano source files.
func (s *Server) LoadProject() error { return s.ag.LoadProject() }

// Start starts the HTTP server.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/clear", s.handleClear)
	mux.HandleFunc("/api/reload", s.handleReload)
	mux.HandleFunc("/api/write", s.handleWrite)
	mux.HandleFunc("/api/diff", s.handleDiff)
	mux.HandleFunc("/api/autowrite", s.handleAutoWrite)

	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("⚡ ZATRANO AI Agent → http://localhost%s\n", addr)

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Minute,
		IdleTimeout:  60 * time.Second,
	}
	return srv.ListenAndServe()
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(indexHTML))
}

// ChatRequest is the JSON body for POST /api/chat.
type ChatRequest struct {
	Message string `json:"message"`
	Mode    string `json:"mode"`
}

// handleChat streams response via SSE, then sends parsed files metadata.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST gerekli", http.StatusMethodNotAllowed)
		return
	}
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Geçersiz JSON", http.StatusBadRequest)
		return
	}
	if req.Message == "" {
		http.Error(w, "message boş olamaz", http.StatusBadRequest)
		return
	}

	mode := agent.Mode(req.Mode)
	if mode == "" {
		mode = agent.ModeGenerate
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE desteklenmiyor", http.StatusInternalServerError)
		return
	}

	sseJSON := func(v any) {
		data, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	ctx := r.Context()
	_, files, err := s.ag.ChatWeb(ctx, mode, req.Message, func(token string) {
		sseJSON(map[string]string{"token": token})
	})

	if err != nil {
		sseJSON(map[string]string{"error": err.Error()})
		return
	}

	// Send parsed file list after streaming
	if len(files) > 0 {
		type fileInfo struct {
			Path string `json:"path"`
			Lang string `json:"lang"`
			Size int    `json:"size"`
		}
		infos := make([]fileInfo, len(files))
		for i, f := range files {
			infos[i] = fileInfo{Path: f.Path, Lang: f.Lang, Size: len(f.Content)}
		}

		// Compute diffs
		diffs := s.ag.DiffFiles(files)
		type diffInfo struct {
			Path  string `json:"path"`
			IsNew bool   `json:"is_new"`
			Adds  int    `json:"adds"`
			Dels  int    `json:"dels"`
			HTML  string `json:"html"`
		}
		diffInfos := make([]diffInfo, len(diffs))
		for i, d := range diffs {
			adds, dels := countKind(d.Hunks, diff.LineAdd), countKind(d.Hunks, diff.LineDelete)
			diffInfos[i] = diffInfo{
				Path:  d.Path,
				IsNew: d.IsNew,
				Adds:  adds,
				Dels:  dels,
				HTML:  diff.HTMLDiff(d),
			}
		}

		sseJSON(map[string]any{
			"files": infos,
			"diffs": diffInfos,
		})
	}

	sseJSON(map[string]bool{"done": true})
}

// WriteRequest is the body for POST /api/write.
type WriteRequest struct {
	Files []struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Lang    string `json:"lang"`
	} `json:"files"`
}

func (s *Server) handleWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST gerekli", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var req WriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if s.projectDir == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "proje dizini ayarlanmamış"})
		return
	}

	files := make([]writer.ParsedFile, len(req.Files))
	for i, f := range req.Files {
		files[i] = writer.ParsedFile{Path: f.Path, Content: f.Content, Lang: f.Lang}
	}

	results, _ := s.ag.WriteFiles(files)

	type result struct {
		Path    string `json:"path"`
		Written bool   `json:"written"`
		Created bool   `json:"created"`
		Error   string `json:"error,omitempty"`
	}
	out := make([]result, len(results))
	for i, r := range results {
		res := result{Path: r.Path, Written: r.Written, Created: r.Created}
		if r.Error != nil {
			res.Error = r.Error.Error()
		}
		out[i] = res
	}
	json.NewEncoder(w).Encode(map[string]any{"results": out})
}

// handleDiff returns HTML diff for a file path + new content.
func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST gerekli", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	files := []writer.ParsedFile{{Path: req.Path, Content: req.Content}}
	diffs := s.ag.DiffFiles(files)
	if len(diffs) == 0 {
		json.NewEncoder(w).Encode(map[string]string{"html": ""})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"html": diff.HTMLDiff(diffs[0])})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	stats := s.ag.GetStats()

	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ollamaOK := s.ag.CheckOllama(pingCtx) == nil

	json.NewEncoder(w).Encode(map[string]any{
		"model":       s.llmCfg.Model,
		"project_dir": s.projectDir,
		"ollama_url":  s.llmCfg.BaseURL,
		"file_count":  stats.FileCount,
		"rag_docs":    stats.RAGDocs,
		"auto_write":  stats.AutoWrite,
		"ollama_ok":   ollamaOK,
	})
}

func (s *Server) handleClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST gerekli", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	s.ag.ClearHistory()
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST gerekli", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if err := s.LoadProject(); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleAutoWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST gerekli", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	var req struct {
		Enabled bool `json:"enabled"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	s.ag.SetAutoWrite(req.Enabled)
	json.NewEncoder(w).Encode(map[string]bool{"auto_write": req.Enabled})
}

func countKind(hunks []diff.Hunk, k diff.LineKind) int {
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
