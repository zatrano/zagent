package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"zatrano-agent/internal/diff"
	"zatrano-agent/internal/llm"
	"zatrano-agent/internal/loader"
	"zatrano-agent/internal/rag"
	"zatrano-agent/internal/ui"
	"zatrano-agent/internal/writer"
)

// Agent orchestrates the LLM client, project loader, RAG index and terminal UI.
type Agent struct {
	llmClient  *llm.Client
	projectCtx *loader.ProjectContext
	ragIndex   *rag.Index
	mode       Mode
	projectDir string
	autoWrite  bool // automatically write generated files to disk
}

// New creates a new Agent.
func New(cfg llm.Config, projectDir string) *Agent {
	return &Agent{
		llmClient:  llm.NewClient(cfg),
		mode:       ModeGenerate,
		projectDir: projectDir,
		autoWrite:  false,
	}
}

// SetAutoWrite enables or disables automatic file writing.
func (a *Agent) SetAutoWrite(v bool) { a.autoWrite = v }

// LoadProject (re)loads source files and rebuilds the RAG index.
func (a *Agent) LoadProject() error {
	if a.projectDir == "" {
		ui.Warn("Proje dizini belirtilmedi — kaynak kodu olmadan çalışılacak.")
		return nil
	}
	ui.Loading(fmt.Sprintf("Proje yükleniyor: %s", a.projectDir))
	ctx, err := loader.Load(a.projectDir)
	if err != nil {
		return fmt.Errorf("proje yüklenemedi: %w", err)
	}
	a.projectCtx = ctx

	ui.Loading("RAG indeksi oluşturuluyor...")
	a.ragIndex = rag.Build(ctx)
	ui.Success(fmt.Sprintf("Proje yüklendi — %d dosya indekslendi\n%s", a.ragIndex.Size(), ctx.Stats()))
	return nil
}

// CheckOllama verifies Ollama is reachable.
func (a *Agent) CheckOllama(ctx context.Context) error {
	return a.llmClient.Ping(ctx)
}

// Run starts the interactive terminal loop.
func (a *Agent) Run(ctx context.Context) {
	for {
		input, err := ui.Prompt(string(a.mode))
		if err != nil {
			break
		}
		if input == "" {
			continue
		}
		if a.handleCommand(ctx, input) {
			continue
		}
		a.chat(ctx, input)
	}
}

func (a *Agent) handleCommand(ctx context.Context, input string) bool {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return false
	}
	cmd := strings.ToLower(fields[0])

	switch cmd {
	case "/yardım", "/help":
		ui.Help()
		return true

	case "/temizle", "/clear":
		a.llmClient.ClearHistory()
		ui.Success("Konuşma geçmişi temizlendi.")
		return true

	case "/istatistik", "/stats":
		if a.projectCtx == nil {
			ui.Warn("Proje yüklü değil.")
		} else {
			ui.Section("Proje İstatistikleri")
			fmt.Println(a.projectCtx.Stats())
			if a.ragIndex != nil {
				fmt.Printf("  RAG indeksi: %d döküman\n", a.ragIndex.Size())
			}
		}
		return true

	case "/yükle", "/reload":
		if err := a.LoadProject(); err != nil {
			ui.Error(err.Error())
		}
		return true

	case "/yaz", "/write":
		a.autoWrite = true
		ui.Success("Otomatik dosya yazma AÇIK — üretilen kodlar diske kaydedilecek.")
		return true

	case "/yazmadur", "/nowrite":
		a.autoWrite = false
		ui.Warn("Otomatik dosya yazma KAPALI.")
		return true

	case "/diff":
		// /diff <dosya yolu>
		if len(fields) < 2 {
			ui.Info("Kullanım: /diff <dosya yolu>")
		} else {
			a.showDiff(strings.TrimSpace(strings.Join(fields[1:], " ")))
		}
		return true

	case "/ara", "/search":
		// /ara <sorgu> — RAG araması
		if len(fields) < 2 {
			ui.Info("Kullanım: /ara <arama terimi>")
		} else {
			query := strings.Join(fields[1:], " ")
			a.ragSearch(query)
		}
		return true

	case "/mod", "/mode":
		a.changeMode(input)
		return true

	case "/yeni":
		a.mode = ModeGenerate
		ui.Info(fmt.Sprintf("Mod: %s", a.mode.ModeLabel()))
		return true

	case "/açıkla", "/explain":
		parts := strings.SplitN(input, " ", 2)
		if len(parts) > 1 {
			a.explainFile(ctx, strings.TrimSpace(parts[1]))
		} else {
			a.mode = ModeExplain
			ui.Info(fmt.Sprintf("Mod: %s", a.mode.ModeLabel()))
		}
		return true

	case "/hata", "/debug":
		a.mode = ModeDebug
		ui.Info(fmt.Sprintf("Mod: %s", a.mode.ModeLabel()))
		return true

	case "/incele", "/review":
		a.mode = ModeReview
		ui.Info(fmt.Sprintf("Mod: %s", a.mode.ModeLabel()))
		return true

	case "/serbest", "/free":
		a.mode = ModeFree
		ui.Info(fmt.Sprintf("Mod: %s", a.mode.ModeLabel()))
		return true

	case "/çıkış", "/exit", "/quit", "/q":
		ui.Success("Görüşürüz!")
		os.Exit(0)
	}
	return false
}

func (a *Agent) changeMode(input string) {
	parts := strings.Fields(input)
	if len(parts) < 2 {
		ui.Info(fmt.Sprintf("Mevcut mod: %s", a.mode.ModeLabel()))
		return
	}
	switch strings.ToLower(parts[1]) {
	case "üret", "uret", "generate":
		a.mode = ModeGenerate
	case "açıkla", "acikla", "explain":
		a.mode = ModeExplain
	case "hata", "debug":
		a.mode = ModeDebug
	case "incele", "review":
		a.mode = ModeReview
	case "serbest", "free":
		a.mode = ModeFree
	default:
		ui.Warn(fmt.Sprintf("Bilinmeyen mod: %s", parts[1]))
		return
	}
	ui.Success(fmt.Sprintf("Mod: %s", a.mode.ModeLabel()))
}

// showDiff reads a file and shows what would change if overwritten with a new version.
func (a *Agent) showDiff(filePath string) {
	fullPath := filePath
	if !filepath.IsAbs(filePath) && a.projectDir != "" {
		fullPath = filepath.Join(a.projectDir, filePath)
	}
	content, err := os.ReadFile(fullPath)
	if err != nil {
		ui.Error(fmt.Sprintf("Dosya okunamadı: %v", err))
		return
	}
	ui.Section(fmt.Sprintf("Diff: %s", filePath))
	lines := strings.Split(string(content), "\n")
	fmt.Printf("  %d satır, %d byte\n\n", len(lines), len(content))
	// Show first 30 lines
	for i, l := range lines {
		if i >= 30 {
			fmt.Printf("  ... (%d satır daha)\n", len(lines)-30)
			break
		}
		fmt.Printf("  %3d  %s\n", i+1, l)
	}
}

// ragSearch shows the top matching files for a query.
func (a *Agent) ragSearch(query string) {
	if a.ragIndex == nil {
		ui.Warn("RAG indeksi yüklü değil. Önce /yükle komutunu çalıştır.")
		return
	}
	ui.Section(fmt.Sprintf("RAG Arama: \"%s\"", query))
	results := a.ragIndex.Search(query, 5)
	for i, doc := range results {
		fmt.Printf("  %d. %s\n", i+1, doc.Path)
	}
	fmt.Println()
}

func (a *Agent) explainFile(ctx context.Context, filePath string) {
	fullPath := filePath
	if !filepath.IsAbs(filePath) && a.projectDir != "" {
		fullPath = filepath.Join(a.projectDir, filePath)
	}
	content, err := os.ReadFile(fullPath)
	if err != nil {
		ui.Error(fmt.Sprintf("Dosya okunamadı: %s (%v)", fullPath, err))
		return
	}
	extra := fmt.Sprintf("**Dosya: %s**\n```go\n%s\n```", filePath, string(content))
	a.chatWithExtra(ctx, "Bu dosyayı detaylı açıkla.", extra, ModeExplain)
}

func (a *Agent) chat(ctx context.Context, userInput string) {
	a.chatWithExtra(ctx, userInput, "", a.mode)
}

func (a *Agent) chatWithExtra(ctx context.Context, userInput, extra string, mode Mode) {
	// Use RAG to build a focused context instead of sending ALL files
	var sourceCode string
	if a.ragIndex != nil {
		sourceCode = a.ragIndex.BuildContext(userInput, 8, 60_000)
	} else if a.projectCtx != nil {
		sourceCode = a.projectCtx.Summary
	}

	systemPrompt := BuildSystemPrompt(sourceCode)
	userMessage := BuildUserMessage(mode, userInput, extra)

	ui.AgentStart()

	response, err := a.llmClient.Chat(ctx, systemPrompt, userMessage, func(token string) {
		ui.Token(token)
	})

	if err != nil {
		ui.Error(fmt.Sprintf("LLM hatası: %v", err))
		ui.Warn("Ollama çalışıyor mu? `ollama serve` komutunu dene.")
		return
	}

	ui.AgentEnd(a.llmClient.HistoryLen())

	// Post-process: parse files from response
	if mode == ModeGenerate && a.projectDir != "" {
		a.handleGeneratedCode(response)
	}
}

// handleGeneratedCode parses generated files, shows diff, and optionally writes.
func (a *Agent) handleGeneratedCode(response string) {
	files := writer.Parse(response)
	if len(files) == 0 {
		return
	}

	ui.Section(fmt.Sprintf("Üretilen Dosyalar (%d adet)", len(files)))
	for _, f := range files {
		fmt.Printf("  📄 %s\n", f.Path)
	}

	// Show diff summary
	diffs := diff.Compute(a.projectDir, files)
	fmt.Println()
	for _, d := range diffs {
		if d.IsNew {
			fmt.Printf("  🆕 %s (yeni dosya, %d satır)\n", d.Path, len(d.NewLines))
		} else if len(d.Hunks) == 0 {
			fmt.Printf("  ✓  %s (değişiklik yok)\n", d.Path)
		} else {
			adds := countLines(d.Hunks, diff.LineAdd)
			dels := countLines(d.Hunks, diff.LineDelete)
			fmt.Printf("  📝 %s (+%d -%d)\n", d.Path, adds, dels)
		}
	}

	if a.autoWrite {
		fmt.Println()
		results := writer.WriteAll(a.projectDir, files)
		fmt.Print(writer.Summary(results))
		fmt.Println()
	} else {
		fmt.Printf("\n  💡 Dosyaları kaydetmek için /yaz komutunu ver, sonra tekrar üret.\n")
	}
}

func countLines(hunks []diff.Hunk, kind diff.LineKind) int {
	n := 0
	for _, h := range hunks {
		for _, l := range h.Lines {
			if l.Kind == kind {
				n++
			}
		}
	}
	return n
}

// ── Web-facing methods ────────────────────────────────────────────────────────

// Stats holds summary info about the loaded project.
type Stats struct {
	FileCount  int    `json:"file_count"`
	RAGDocs    int    `json:"rag_docs"`
	ProjectDir string `json:"project_dir"`
	AutoWrite  bool   `json:"auto_write"`
}

// GetStats returns current project statistics.
func (a *Agent) GetStats() Stats {
	s := Stats{ProjectDir: a.projectDir, AutoWrite: a.autoWrite}
	if a.projectCtx != nil {
		s.FileCount = len(a.projectCtx.Files)
	}
	if a.ragIndex != nil {
		s.RAGDocs = a.ragIndex.Size()
	}
	return s
}

// ClearHistory resets the LLM conversation history.
func (a *Agent) ClearHistory() { a.llmClient.ClearHistory() }

// ChatWeb is the web-facing chat method with streaming.
// It returns the full response and the parsed file list.
func (a *Agent) ChatWeb(ctx context.Context, mode Mode, userInput string, onToken llm.StreamToken) (string, []writer.ParsedFile, error) {
	var sourceCode string
	if a.ragIndex != nil {
		sourceCode = a.ragIndex.BuildContext(userInput, 8, 60_000)
	} else if a.projectCtx != nil {
		sourceCode = a.projectCtx.Summary
	}

	systemPrompt := BuildSystemPrompt(sourceCode)
	userMessage := BuildUserMessage(mode, userInput, "")

	response, err := a.llmClient.Chat(ctx, systemPrompt, userMessage, onToken)
	if err != nil {
		return "", nil, err
	}

	var files []writer.ParsedFile
	if mode == ModeGenerate && a.projectDir != "" {
		files = writer.Parse(response)
	}
	return response, files, nil
}

// WriteFiles writes the given files to the project directory and returns diffs.
func (a *Agent) WriteFiles(files []writer.ParsedFile) ([]writer.WriteResult, []diff.FileDiff) {
	diffs := diff.Compute(a.projectDir, files)
	results := writer.WriteAll(a.projectDir, files)
	return results, diffs
}

// DiffFiles computes diffs without writing.
func (a *Agent) DiffFiles(files []writer.ParsedFile) []diff.FileDiff {
	return diff.Compute(a.projectDir, files)
}
