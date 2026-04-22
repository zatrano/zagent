# ⚡ ZATRANO AI Agent

Zatrano projesinin **gerçek kaynak kodunu** okuyarak çalışan local AI geliştirici asistanı.
RAG (Retrieval-Augmented Generation) ile akıllı dosya seçimi, otomatik diff ve doğrudan diske yazma desteği.

---

## Nasıl Çalışır?

```
Zatrano .go dosyaları
        ↓  loader okur
TF-IDF RAG indeksi oluşturulur
        ↓  sorguya göre ilgili dosyalar seçilir
Ollama'ya sistem prompt olarak gönderilir
        ↓
Gerçek koda dayalı, mimariye uygun kod üretilir
        ↓
Diff gösterilir → onaylarsanız diske yazılır
```

---

## Kurulum

### 🪟 Windows

#### 1. Go Kur
- https://go.dev/dl/ adresinden **Windows amd64 installer** (.msi) indir
- Kur, **Restart** et (PATH otomatik eklenir)
- Doğrula: `go version`

#### 2. Ollama Kur
- https://ollama.com/download adresinden **Windows** sürümünü indir
- `.exe` kurulumunu çalıştır
- Ollama sistem tepsisine oturur, otomatik başlar
- Doğrula: `ollama --version`

#### 3. Modeli İndir
```powershell
# 8GB RAM için (önerilen)
ollama pull qwen2.5-coder:7b

# 16GB RAM için
ollama pull qwen2.5-coder:14b
```

#### 4. Agent'ı Derle
```powershell
git clone https://github.com/zatrano/zatrano-agent.git
cd zatrano-agent
go mod tidy
go build -o zatrano-agent.exe ./cmd/agent
```

#### 5. Çalıştır
```powershell
# Terminal modu
.\zatrano-agent.exe -proje C:\Users\kullanici\zatrano

# Web modu (tarayıcı: http://localhost:8080)
.\zatrano-agent.exe -proje C:\Users\kullanici\zatrano -web

# Ortam değişkeniyle
$env:ZATRANO_PROJECT = "C:\Users\kullanici\zatrano"
.\zatrano-agent.exe -web
```

---

### 🐧 Linux

#### 1. Go Kur
```bash
# Ubuntu / Debian
sudo apt update && sudo apt install -y golang-go

# Veya manuel (daha güncel versiyon için)
wget https://go.dev/dl/go1.23.4.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.23.4.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Doğrula
go version
```

#### 2. Ollama Kur
```bash
curl -fsSL https://ollama.com/install.sh | sh

# Servis olarak başlat
sudo systemctl enable ollama
sudo systemctl start ollama

# Doğrula
ollama --version
```

#### 3. Modeli İndir
```bash
# 8GB RAM için (önerilen)
ollama pull qwen2.5-coder:7b

# 16GB RAM için
ollama pull qwen2.5-coder:14b
```

#### 4. Agent'ı Derle
```bash
git clone https://github.com/zatrano/zatrano-agent.git
cd zatrano-agent
go mod tidy
go build -o zatrano-agent ./cmd/agent
```

#### 5. Çalıştır
```bash
# Terminal modu
./zatrano-agent -proje /home/kullanici/zatrano

# Web modu (tarayıcı: http://localhost:8080)
./zatrano-agent -proje /home/kullanici/zatrano -web

# Makefile ile
make run PROJE=/home/kullanici/zatrano
make run-web PROJE=/home/kullanici/zatrano

# Ortam değişkeniyle
export ZATRANO_PROJECT=/home/kullanici/zatrano
./zatrano-agent -web
```

---

## Model Seçimi (RAM'e Göre)

| RAM   | Model                  | İndirme Komutu                        | Boyut  |
|-------|------------------------|---------------------------------------|--------|
| 8GB   | `qwen2.5-coder:7b`     | `ollama pull qwen2.5-coder:7b`        | ~4.7GB |
| 16GB  | `qwen2.5-coder:14b`    | `ollama pull qwen2.5-coder:14b`       | ~9GB   |
| 32GB+ | `qwen2.5-coder:32b`    | `ollama pull qwen2.5-coder:32b`       | ~20GB  |

---

## Flags

```
-model   Ollama model adı           (varsayılan: qwen2.5-coder:7b)
-url     Ollama URL                 (varsayılan: http://localhost:11434)
-proje   Zatrano proje dizini
-temp    Sıcaklık 0.0-1.0           (varsayılan: 0.2)
-web     Web arayüzü modunu aç
-port    Web sunucu portu           (varsayılan: 8080)
```

---

## Özellikler

### 🧠 RAG (Retrieval-Augmented Generation)
Proje yüklendiğinde tüm `.go` dosyaları TF-IDF ile indekslenir.
Her soruda sadece **ilgili dosyalar** model bağlamına eklenir — 8GB RAM'de bile verimli çalışır.

### 📊 Diff Görünümü
Üretilen her dosya için mevcut versiyon ile karşılaştırma yapılır:
- 🆕 Yeni dosyalar işaretlenir
- `+N / -N` satır değişikliği gösterilir
- Web arayüzünde satır satır diff paneli açılır

### 💾 Dosya Yazma
İki mod:
- **Manuel:** Her dosyanın yanındaki **Yaz** butonuna bas
- **Auto-Write:** Sidebar'daki toggle ile aç — üretilen kodlar otomatik kaydedilir

### 🔌 API Endpoint'leri
```
GET  /              Web arayüzü
POST /api/chat      SSE streaming chat
GET  /api/status    Model ve proje durumu
POST /api/clear     Konuşma geçmişi sıfırla
POST /api/reload    Projeyi yeniden yükle
POST /api/write     Dosyaları diske yaz
POST /api/diff      Tek dosya diff HTML
POST /api/autowrite Auto-write toggle
```

---

## Terminal Komutları

| Komut                | Açıklama                              |
|----------------------|---------------------------------------|
| `/yeni`              | Kod üretme moduna geç                 |
| `/açıkla <dosya>`    | Belirtilen dosyayı açıkla             |
| `/hata`              | Hata bulma moduna geç                 |
| `/incele`            | Kod inceleme moduna geç               |
| `/serbest`           | Serbest sohbet modu                   |
| `/yaz`               | Auto-write AÇIK                       |
| `/yazmadur`          | Auto-write KAPALI                     |
| `/diff <dosya>`      | Dosyayı terminalde göster             |
| `/ara <sorgu>`       | RAG ile dosya ara                     |
| `/temizle`           | Konuşma geçmişini sıfırla             |
| `/istatistik`        | Yüklü dosya ve RAG istatistikleri     |
| `/yükle`             | Projeyi yeniden yükle                 |
| `/yardım`            | Bu yardımı göster                     |
| `/çıkış`             | Çık                                   |

---

## Örnek Kullanım

```
[üret] ➤ Product adinda modul olustur. Fields: Name string, Price float64, StockCount int

⚡ ZATRANO AI:
──────────────────────────────────────────────
// models/product.go
```go
package models

// Product represents a product in the catalog.
type Product struct {
    BaseModel
    Name       string  `gorm:"size:200;not null;index"`
    Price      float64 `gorm:"not null"`
    StockCount int     `gorm:"default:0"`
}

var _ = (*Product)(nil)
```

// repositories/product_repository.go
...

⚡ 4 DOSYA ÜRETİLDİ
  🆕 models/product.go (yeni dosya, 12 satır)
  🆕 repositories/product_repository.go (+45)
  🆕 services/product_service.go (+67)
  📝 app/container.go (+8 -0)
```

---

## Makefile Komutları

```bash
make build                        # Derle
make run   PROJE=../zatrano       # Terminal modu
make run-web PROJE=../zatrano     # Web modu
make tidy                         # go mod tidy
make clean                        # Binary sil
```

---

## Lisans

MIT © 2026 — ZATRANO
