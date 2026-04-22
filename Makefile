.PHONY: build run clean tidy

# Değişkenler
BINARY    = zatrano-agent
CMD       = ./cmd/agent
PROJE    ?= ../zatrano
MODEL    ?= qwen2.5-coder:7b

## build: Derleme
build:
	go build -o $(BINARY) $(CMD)
	@echo "✓ $(BINARY) derlendi"

## run: Derle ve çalıştır
run: build
	./$(BINARY) -proje $(PROJE) -model $(MODEL)

## run-no-project: Proje olmadan çalıştır (test için)
run-no-project: build
	./$(BINARY) -model $(MODEL)

## tidy: Bağımlılıkları düzenle
tidy:
	go mod tidy

## clean: Derlenmiş dosyayı sil
clean:
	rm -f $(BINARY)

## help: Yardım
help:
	@grep -E '^## ' Makefile | sed 's/## /  /'

## run-web: Web modunda çalıştır
run-web: build
	./$(BINARY) -proje $(PROJE) -model $(MODEL) -web -port 8080
