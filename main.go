// Zatrano AI Agent - Zatrano framework'e özel local AI geliştirici asistanı.
//
// Kullanım:
//
//	zatrano-agent [seçenekler]
//
// Bayraklar:
//
//	-model    Ollama model adı (varsayılan: qwen2.5-coder:7b)
//	-url      Ollama sunucu URL'si (varsayılan: http://localhost:11434)
//	-proje    Zatrano proje dizini
//	-temp     Sıcaklık 0.0-1.0 (varsayılan: 0.2)
//	-web      Web arayüzü modunu aç
//	-port     Web sunucu portu (varsayılan: 8080)
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"zatrano-agent/internal/agent"
	"zatrano-agent/internal/llm"
	"zatrano-agent/internal/ui"
	"zatrano-agent/internal/web"
)

func main() {
	model     := flag.String("model", "qwen2.5-coder:7b", "Ollama model adı")
	ollamaURL := flag.String("url", "http://localhost:11434", "Ollama sunucu URL")
	projectDir := flag.String("proje", "", "Zatrano proje dizini")
	temp      := flag.Float64("temp", 0.2, "Sıcaklık (0.0-1.0)")
	webMode   := flag.Bool("web", false, "Web arayüzü modunu aç")
	port      := flag.Int("port", 8080, "Web sunucu portu")
	flag.Parse()

	// Ortam değişkeninden de al
	if *projectDir == "" {
		if env := os.Getenv("ZATRANO_PROJECT"); env != "" {
			*projectDir = env
		}
	}

	// Mutlak yola çevir
	if *projectDir != "" {
		if abs, err := filepath.Abs(*projectDir); err == nil {
			*projectDir = abs
		}
	}

	cfg := llm.Config{
		BaseURL:     *ollamaURL,
		Model:       *model,
		Temperature: *temp,
		NumCtx:      4096,
	}

	if *webMode {
		runWeb(cfg, *projectDir, *port)
	} else {
		runCLI(cfg, *projectDir)
	}
}

func runCLI(cfg llm.Config, projectDir string) {
	ui.Banner(cfg.Model, orDefault(projectDir, "(belirtilmedi)"))
	ui.Help()

	a := agent.New(cfg, projectDir)

	ui.Loading("Ollama bağlantısı kontrol ediliyor")
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.CheckOllama(pingCtx); err != nil {
		ui.Error(fmt.Sprintf("Ollama bağlantısı kurulamadı: %v", err))
		ui.Warn("Çözüm:")
		fmt.Println("  1. ollama serve")
		fmt.Printf("  2. ollama pull %s\n", cfg.Model)
		fmt.Println("  3. Bu programı tekrar çalıştır")
		os.Exit(1)
	}
	ui.Success("Ollama bağlı")

	if err := a.LoadProject(); err != nil {
		ui.Warn(fmt.Sprintf("Proje yüklenemedi: %v", err))
		ui.Warn("Kaynak kodu olmadan devam ediliyor...")
	}

	ui.Section("Hazır — yazmaya başla")
	a.Run(context.Background())
}

func runWeb(cfg llm.Config, projectDir string, port int) {
	fmt.Printf("\n⚡ Zatrano AI Agent — Web Modu\n")
	fmt.Printf("   Model:  %s\n", cfg.Model)
	fmt.Printf("   Proje:  %s\n\n", orDefault(projectDir, "(belirtilmedi)"))

	srv := web.NewServer(cfg, projectDir, port)

	if projectDir != "" {
		fmt.Print("🔄 Proje yükleniyor...")
		if err := srv.LoadProject(); err != nil {
			fmt.Printf("\n⚠️  Proje yüklenemedi: %v\n", err)
		} else {
			fmt.Println(" ✓")
		}
	}

	if err := srv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Sunucu başlatılamadı: %v\n", err)
		os.Exit(1)
	}
}

func orDefault(val, def string) string {
	if val == "" {
		return def
	}
	return val
}
