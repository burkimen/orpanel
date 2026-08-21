# OmniRoute Kontrol Paneli (orPanel)

[OmniRoute](https://github.com/TorhunORG/OmniRoute) için çapraz platform masaüstü kontrol paneli — AI istek yönlendiricisi ve load balancer. orPanel sistem tepsisinde çalışır, yerleşik terminal, sağlık izleme ve OmniRoute için tek tıkla kurulum/güncelleme sunar.

> OmniRoute topluluğu için ❤️ ile yapıldı.

## Özellikler

- **Sistem Tepsisi** — Görev çubuğundan her an erişilebilir
- **Yerleşik Terminal** — VS Code tarzı yeniden boyutlandırılabilir alt panel
- **Sağlık İzleme** — Gerçek zamanlı OmniRoute durumu, sürüm kontrolü, tek tıkla kurulum/güncelleme/onarım
- **Tema Sistemi** — Açık, Koyu, Sistem (otomatik algılama)
- **Çoklu Dil** — Türkçe, İngilizce, İspanyolca (genişletilebilir)
- **Çapraz Platform** — Windows, macOS, Linux
- **Yönetici hakki gerektirmez** — Taşınabilir veya XDG uyumlu kurulum

## Kurulum

### 1. npm (önerilen)

```bash
npm install -g orpanel
orpanel
```

### 2. bun

```bash
bun add -g orpanel
bunx orpanel
```

### 3. curl (Linux/macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/burkimen/orpanel/main/scripts/install/install.sh | sh
```

### 4. PowerShell (Windows)

```powershell
irm https://raw.githubusercontent.com/burkimen/orpanel/main/scripts/install/install.ps1 | iex
```

### 5. Homebrew

```bash
brew tap burkimen/orpanel
brew install orpanel
```

### 6. Nix

```bash
nix profile install github:burkimen/orpanel
```

### 7. mise

```bash
mise use -g ubi:burkimen/orpanel
```

## Hızlı Başlangıç

```bash
orpanel           # Kontrol panelini başlat
orpanel --help    # Yardımı göster
```

Tarayıcınızda `http://localhost:20127` adresini açın. Panel şunları yapar:

1. Sisteminizdeki OmniRoute'u algılar
2. Sürüm bilgisiyle sağlık durumunu gösterir
3. OmniRoute kurulu değilse tek tıkla kurulum önerir
4. Yerleşik terminalde OmniRoute loglarını gösterir

## OmniRoute Entegrasyonu

orPanel [OmniRoute](https://github.com/TorhunORG/OmniRoute) ile sorunsuz çalışır:

- **Otomatik algılama** — `npm prefix -g`, NVM, sistem yollarından OmniRoute'u bulur
- **Sağlık kontrolü** — `GET /api/omni/health` sürüm, port, node uyumluluğu izler
- **Tek tıkla işlemler** — Web arayüzünden Kur, Güncelle, Onar, Yeniden Kur
- **Canlı terminal** — npm çıktısı yerleşik xterm'e akar
- **Sürüm takibi** — Yerel vs npm registry son sürüm karşılaştırması, güncelleme uyarısı

## Geliştirme

```bash
git clone https://github.com/burkimen/orpanel.git
cd orpanel
go build -ldflags "-H=windowsgui" -o orPanel.exe   # Windows
go build -o orPanel                                  # Linux/macOS
```

Go 1.22+ gerektirir. Binary ile `themes/`, `locales/` dizinleri aynı yerde olmalıdır.

## Lisans

[MIT](LICENSE) — [OmniRoute](https://github.com/TorhunORG/OmniRoute) topluluğu için yapıldı.
