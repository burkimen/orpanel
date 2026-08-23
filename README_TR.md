# OmniRoute Kontrol Paneli (orPanel)

[OmniRoute](https://github.com/TorhunORG/OmniRoute) icin capraz platform masaustu kontrol paneli — AI istek yonlendiricisi ve load balancer. orPanel sistem tepsisinde calisir, yerlesik terminal, saglik izleme ve OmniRoute icin tek tikla kurulum/guncelleme sunar.

> OmniRoute toplulugu icin ❤️ ile yapildi.

<p align="center">
  <img src="./screenshot.png" alt="orPanel Durum" width="100%">
</p>

<p align="center">
  <img src="./screenshot_settings.png" alt="orPanel Ayarlar" width="100%">
</p>

## Ozellikler

- **Sistem Tepsisi** — Gorev cubugundan her an erisilebilir
- **Yerlesik Terminal** — VS Code tarzi yeniden boyutlandirilabilir alt panel
- **Saglik Izleme** — Gercek zamanli OmniRoute durumu, surum kontrolu, tek tikla kurulum/guncelleme/onarim
- **Tema Sistemi** — Acik, Koyu, Sistem (otomatik algilama)
- **Coklu Dil** — Turkce, Ingilizce, Ispanyolca (genisletilebilir)
- **Capraz Platform** — Windows, macOS, Linux
- **Bagimlik gerektirmez** — Tek dosya, her sey icinde gomulu

## Kurulum

### Unix (Linux/macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/burkimen/orpanel/main/scripts/install/install.sh | sh
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/burkimen/orpanel/main/scripts/install/install.ps1 | iex
```

### Guncelleme

Ayni komutu tekrar calistir — en son surumu otomatik indirir.

### Kaldirma

```bash
# Linux/macOS
rm ~/.local/bin/orpanel

# Windows
Remove-Item "$env:LOCALAPPDATA\Programs\Orpanel\orPanel.exe"
```

## Kullanim

```bash
orpanel              # Interaktif CLI menusu
orpanel --web        # Web UI'yi tarayicida ac
orpanel --tray       # Sistem tepsisinde baslat
orpanel --version    # Surumu goster
orpanel --help       # Yardimi goster
```

## Hizli Baslangic

Tarayicinizda `http://localhost:20127` adresini acin. Panel sunlari yapar:

1. Sisteminizdeki OmniRoute'u algilar
2. Surum bilgisiyle saglik durumunu gosterir
3. OmniRoute kurulmadisa tek tikla kurulum onerir
4. Yerlesik terminalde OmniRoute loglarini gosterir

## OmniRoute Entegrasyonu

orPanel [OmniRoute](https://github.com/TorhunORG/OmniRoute) ile sorunsuz calisir:

- **Otomatik algilama** — `npm prefix -g`, NVM, sistem yollarindan OmniRoute'u bulur
- **Saglik kontrolu** — `GET /api/omni/health` surum, port, node uyumlulugu izler
- **Tek tikla islemler** — Web arayuzunden Kur, Guncelle, Onar, Yeniden Kur
- **Yerlesik terminal** — npm ciktisi yerlesik xterm'e aker
- **Surum takibi** — Yerel vs npm registry son surum karsilastirmasi, guncelleme uyarisi

## Gelistirme

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
├── panel.go           # HTTP sunucusu, sistem tepsisi, surec yonetimi
├── cli.go             # CLI menusu
├── cli_windows.go     # Windows detachli process
├── config.go          # Config yukleme/kaydetme, i18n
├── paths.go           # Capraz platform yol cozumleme
├── omni_health.go     # OmniRoute saglik kontrolleri
├── omni_install.go    # npm kurulum/guncelleme/onarim islemleri
├── autostart_*.go     # Platforma ozel otomatik baslatma
├── dialog_*.go        # Platforma ozel diyaloglar
├── web/               # HTML sablonlari, JS modulleri (gomulu)
├── themes/            # CSS degiskenleri (dark/light)
├── locales/           # i18n dizgileri (tr/en/es)
└── scripts/install/   # curl/irm kurulum scriptleri
```

## Lisans

[MIT](LICENSE) — [OmniRoute](https://github.com/TorhunORG/OmniRoute) toplulugu icin yapildi.
