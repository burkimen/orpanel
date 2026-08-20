# Orpanel Anayasası — Atomik Yapı ve Geçilemez Sınırlar

> **Bu belge projenin anayasasıdır. 5 madde geçilemez sınır çizgisidir. Esnetilemez, ertelenemez, taviz verilemez. Her commit, her PR, her dosya bu anayasaya göre denetlenir.**

**Yürürlük:** 20 Ağustos 2026 — `ae02b66` sonrası tüm kod bu anayasaya tabidir.  
**Kapsam:** `D:\Projects\orpanel\` altındaki tüm kaynak dosyalar.

---

## Madde 1 — Kritik Olmayan Dosyalar: ≤300 Satır

**Kural:** Proje içerisindeki kritik olmayan bütün dosyalar, istisnasız **300 satırı kesinlikle geçmeyecek**.

- **Geçerse ne olur?** Dosya mantıklı bir şekilde, her biri yine **300 satırı geçmeyecek** şekilde modüler parçalara bölünecek.
- **Nasıl bölünür?** Tek sorumluluk prensibi (SRP). Örnek: `panel.go:321` içindeki `htmlTemplate` (500 satır) → `web/templates/index.html` + `web/static/js/health.js` + `web/static/js/theme.js` + `web/static/js/app.js`. Her biri <300.
- **Ölçüm:** `wc -l <file>` veya `Get-Content <file> | Measure-Object -Line`. CI hook: `if ((Get-Content $f).Count -gt 300) { exit 1 }`
- **Örnek ihlal:** `themes/common.css:288` → 288 OK. `panel.go:1408` → **ihlâl** (Madde 2’ye girer, ama kritik olmayan olsaydı zaten 300’ü aştığı için bölünürdü).

---

## Madde 2 — Çok Kritik ve Bölünemez Dosyalar: ≤800 Satır

**Kural:** Çok kritik ve bölünemez dosyalar ise, istisnasız **800 satırı kesinlikle geçmeyecek**.

- **Geçerse ne olur?** Mantıklı bir şekilde modüler olarak yine **300 satırı geçmeyecek** şekilde bölünecek. Yani 800’lük monolit → 3× ~270 satırlık modül.
- **“Bölünemez” ne demek?** Sadece `main.go` gibi gerçekten tek sorumluluğu olan, bölündüğünde anlamını yitirecek dosyalar. Şüphe varsa **Madde 1** uygulanır (300).
- **Mevcut ihlâl:** `panel.go:1408` → **800’ü aştı** → acil bölünecek: `main.go` (<150) + `config.go` (<200) + `server.go` (<300) + `tray.go` (<250) + `watchdog.go` (<250) + `paths.go:191` OK + `omni_health.go:403` → `omni/health.go` + `omni/install.go`.
- **Ölçüm:** Aynı `wc -l`, limit 800. CI’de iki eşik kontrol edilir: `>300` uyarı, `>800` **fail**.

---

## Madde 3 — Metinler Yalnızca `@locales\`

**Kural:** Metin içerikli tüm veriler yalnızca `@locales\` dizinindeki dil sisteminde tutulacak ve gerek duyulduğunda buradan kullanılacak. **Hardcoded text string YASAK.**

- **Kapsar:** HTML içi `OmniRoute Durumu`, JS içi `isEn ? "Running" : "Çalışıyor"`, Go içi `writeLog("INFO: Port %d dolu")` dışındaki kullanıcıya gösterilen her `string`. Sadece log seviyesi (`INFO`, `WARN`) ve teknik anahtarlar istisna olabilir, kullanıcı metni değil.
- **Nereden kullanılır?** 
  - Go template: `{{.T.OmniHealthTitle}}` (`panel.go:383` → `{{.T.OmniHealthTitle}}`)
  - JS: `const T = JSON.parse('{{.TJson}}'); T.OmniHealthBtnInstall` (`panel.go:596` `isEn ?` silinir)
  - Go log/user-msg: `t["LogStarting"]` (`locales/tr.json:31`)
- **Doğrulama:** `grep -R "isEn ?" panel.go` → 0 olmalı. `grep -P '"[A-ZÇĞİÖŞÜ].*"' web/templates/*.html` → sadece `{{.T.`

**Mevcut ihlâl:** `panel.go:606` `Kurulu:` / `Installed:` , `panel.go:643` `OmniRoute Kurulumunu Şimdi Başlat` , `omni_health.go:240` `OmniRoute kurulum` → hepsi `locales/tr|en|es.json`’a taşınacak.

---

## Madde 4 — Renk/Stil/Tema Yalnızca `@themes\`

**Kural:** Renk, stil, tema ve benzeri tüm ihtiyaçlar yalnızca `@themes\` dizinindeki tema sisteminde tutulacak ve gerek duyulduğunda buradan kullanılacak.

- **Kapsar:** `panel.go:321` içindeki `<style>` YASAK (zaten kaldırıldı), `common.css:3` `dark/variables.css:1` `light/variables.css:1` dışındaki hex/rgba/hsl, `font-family`, `grid`, `shadow` hardcod’u YASAK.
- **Nereden kullanılır?** `var(--color-primary)` `var(--grid-line)` `var(--radius)` gibi CSS değişkenleri. Yeni renk ihtiyacı → `themes/dark/variables.css` ve `light`’a eklenir, `common.css`’te `var()` ile kullanılır.
- **Doğrulama:** `grep -R "#[0-9a-fA-F]\{3,6\}" web/ --include="*.html" --include="*.js"` → 0 olmalı. Sadece `themes/*.css`’te hex olabilir.

---

## Madde 5 — Anayasa Geçilemez

**Kural:** Bu maddelerin tamamı bizim anayasamız ve geçilemez sınır çizgilerimizdir. **Kesinlikle esnetilemez, ertelenemez ve taviz verilemez.**

- Her commit öncesi `pre-commit` hook: `wc -l`, `hardcoded` grep, `hex` grep.
- Her PR’da `rokbe-reviewer` bu 5 maddeyi kontrol eder.
- İhlâl eden PR **merge edilmez**, ihlâl eden commit **amend** edilir.
- Bu belge `ANAYASA.md` kök dizinde saklanır, değişiklikleri sadece `anayasa` etiketiyle ve oybirliğiyle yapılır.

---

## Ek — Atomik Yapı

```
orpanel/
├── ANAYASA.md                # bu belge
├── main.go                   # <150  sadece systray.Run
├── config.go                 # <200  Config struct + load/save
├── server.go                 # <300  startWebServer + /api/* handler
├── tray.go                   # <250  onReady, mToggle disable logic
├── watchdog.go               # <250  startWatchdog, backoff
├── paths.go                  # 191   exeDir, getOmniroutePathEnhanced
├── omni/
│   ├── health.go             # <300  checkOmniHealth, isOmniOpRunning
│   └── install.go            # <300  runNpmOmni, install/update/repair
├── web/
│   ├── templates/
│   │   └── index.html        # <300  sadece HTML + {{.T.*}}
│   └── static/
│       ├── js/
│       │   ├── app.js        # <300  switchTab, term, fit
│       │   ├── health.js     # <300  loadOmniHealth/renderHealth
│       │   ├── theme.js      # <300  applyTheme
│       │   └── settings.js   # <300  lang, autostart
│       └── css/              # (themes zaten var)
├── themes/
│   ├── common.css            # 288  OK
│   ├── dark/variables.css
│   └── light/variables.css
└── locales/
    ├── tr.json
    ├── en.json
    └── es.json
```

**Söz:** Bu anayasayı onaylıyorum, her satırda uygulayacağım.

— `burkimen <mburaksaglik@gmail.com>` — 20 Ağustos 2026
