# OmniRoute Kontrol Paneli (orPanel)

[OmniRoute](https://github.com/TorhunORG/OmniRoute) için çapraz platform masaüstü kontrol paneli — AI istek yönlendiricisi ve load balancer. orPanel sistem tepsisinde çalışır, yerleşik terminal, sağlık izleme ve OmniRoute için tek tıkla kurulum/güncelleme sunar.

> OmniRoute topluluğu için ❤️ ile yapıldı.

<p align="center">
  <img src="./screenshot.png" alt="orPanel Durum" width="100%">
</p>

<p align="center">
  <img src="./screenshot_settings.png" alt="orPanel Ayarlar" width="100%">
</p>

## Özellikler

- **Sistem Tepsisi** — Görev çubuğundan her an erişilebilir
- **Yerleşik Terminal** — VS Code tarzı yeniden boyutlandırılabilir alt panel
- **Sağlık İzleme** — Gerçek zamanlı OmniRoute durumu, sürüm kontrolü, tek tıkla kurulum/güncelleme/onarım
- **Tema Sistemi** — Açık, Koyu, Sistem (otomatik algılama)
- **Çoklu Dil** — Türkçe, İngilizce, İspanyolca (genişletilebilir)
- **Çapraz Platform** — Windows, macOS, Linux
- **Bağımlılık gerektirmez** — Tek dosya, her şey içinde gömülü

## Kurulum

### Unix (Linux/macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/burkimen/orpanel/main/scripts/install/install.sh | sh
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/burkimen/orpanel/main/scripts/install/install.ps1 | iex
```

### Güncelleme

Aynı komutu tekrar çalıştır — en son sürümü otomatik indirir.

### Kaldırma

```bash
# Linux/macOS
rm ~/.local/bin/orpanel

# Windows
Remove-Item "$env:LOCALAPPDATA\Programs\Orpanel\orPanel.exe"
```

## Kullanım

```bash
orpanel              # Etkileşimli CLI menüsü
orpanel --web        # Web UI'yi tarayıcıda aç
orpanel --tray       # Sistem tepsisinde başlat
orpanel --version    # Sürümü göster
orpanel --help       # Yardımı göster
```

## Hızlı Başlangıç

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
- **Yerleşik terminal** — npm çıktısı yerleşik xterm'e akar
- **Sürüm takibi** — Yerel vs npm registry son sürüm karşılaştırması, güncelleme uyarısı

## Geliştirme

```bash
git clone https://github.com/burkimen/orpanel.git
cd orpanel
go build -ldflags "-X main.AppVersion=dev" -o orPanel   # Linux/macOS
go build -ldflags "-X main.AppVersion=dev" -o orPanel.exe  # Windows
```

Go 1.23+ gerektirir.

## Mimari

```
orpanel/
├── panel.go           # HTTP sunucusu, sistem tepsisi, süreç yönetimi
├── cli.go             # CLI menüsü
├── cli_windows.go     # Windows ayrılmış süreç
├── cli_unix.go        # Unix stub
├── config.go          # Config yükleme/kaydetme, i18n
├── paths.go           # Çapraz platform yol çözümleme
├── omni_health.go     # OmniRoute sağlık kontrolleri
├── omni_install.go    # npm kurulum/güncelleme/onarım işlemleri
├── autostart_*.go     # Platforma özel otomatik başlatma
├── dialog_*.go        # Platforma özel diyaloglar
├── web/               # HTML şablonları, JS modülleri (gömmeli)
├── themes/            # CSS değişkenleri (dark/light)
├── locales/           # i18n dizgileri (tr/en/es)
└── scripts/install/   # curl/irm kurulum scriptleri
```

## Lisans

[MIT](LICENSE) — [OmniRoute](https://github.com/TorhunORG/OmniRoute) topluluğu için yapıldı.
