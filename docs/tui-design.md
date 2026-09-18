# TUI Tasarım Önerisi (v1.4.5 sonrası — ONAY BEKLİYOR)

Hedef: sahibinin reddettiği ekran yerine geçecek somut tasarım. Kararlar önce,
mockup'lar sonra. Etiketler gerçek Türkçe dizgilerle yazıldı.

## 1. Yerleşim (pane haritası)

Sabit krom: 1 satır başlık (uygulama · sürüm · dil/tema · saat) + 1 satır
durumsal alt bilgi. Gerisi veri panelleri. Oranlar:

| Boyut  | Başlık | Gövde | Alt bilgi | Gövde dağılımı |
|---|---|---|---|---|
| 120x30 | 1 | 28 (sol 40: Durum 8 + İşlemler 20 · sağ 80: Kayıtlar 28) | 1 | iki sütun |
| 80x24 | 1 | Durum 8 + İşlemler 7 + Kayıtlar 7 | 1 | üst üste |
| 60x20 | 1 | odaktaki panel 18 (`Kayıtlar 2/3` yazar) | 1 | tek panel, Tab ile dön |

- Odak göstergesi renk bağımsız: odaktaki panel çift çizgili kenarlık
  (`╔═╗`) + başlıkta `►` işareti; diğerleri tek çizgi. Renk kapalıysa da çalışır.
- Etiket sütunu 11 hücre sabit (`Versiyon  `, `yönetim   ` hizalı).
- Kayıtlar başlığı kaydırma göstergesi taşır: `Kayıtlar ▲ 42/48 ▼`.
  Boş panel asla boş kutu değil: `(kayıt yok — panel yeni başladı)` satırı.
- İşlemler artık tek satırlık çip satırı değil; ok/fare ile gezilen dikey liste.

## 2. Bağlamsal eylemler (durum modeli)

| Durum | Birincil | İkincil / Ayar | Onay (`*`) |
|---|---|---|---|
| çalışıyor | Durdur, Yeniden Başlat | Güncelle (varsa ★ en üstte), Onar, Web Arayüzü, Otomatik Başlat, Dil, Tema | Durdur, Yeniden Başlat, Güncelle, Onar |
| durdu | Başlat | Güncelle, Onar, Web Arayüzü, Otomatik Başlat, Dil, Tema | Güncelle, Onar |
| başlıyor / işlem sürüyor | — (`işlem sürüyor…` satırı) | Web Arayüzü, Dil | — |
| kurulu değil | Kur | Web Arayüzü, Dil, Tema | — |
| ulaşılamıyor | Başlat (yeniden dene) | Onar, Dil | Onar |

- Kural: birbirini tutmayan eylemler GİZLENİR (Başlat ile Durdur asla aynı
  karede olmaz — ret gerekçesinin çekirdeği). Geçici durumlarda (işlem
  sürüyor) liste yerinde kalır, satır `… (işlem sürüyor)` diye pasifleşir.
  Sebep: çelişkili çift kalıcı kafa karışıklığıdır, geçici pasiflik ise
  bilgi verir ve yerleşimi zıplatmaz.
- Taşma: liste kaydırılabilir; sığmayan çipler tek tek düşer ve sonda
  `… +N daha ↓` yazar (sessiz kırpma yok).
- Yardım ekranı o anki duruma erişilebilen TÜM eylemleri listeler
  (aynı tablodan üretilir; tabloda yoksa ekranda yok).

## 3. Fare modeli

- Hover satırı `›` işareti + kalın gösterir; seçim `►` + ters video.
  İkisi aynı anda görünebilir (seçim Yeniden Başlat'ta, hover Güncelle'de).
- Tık seçer; yıkıcı olmayanda ikinci tık / çift tık çalıştırır, yıkıcıda
  onay penceresi açar. Tekerlek Kayıtlar/Yardım'ı kaydırır. Panele tıklamak
  odağı taşır. Modal düğmeleri tıklanabilir; varsayılan `onayla` (Enter).
- Platform gerçeği: tview/tcell'de fare `EnableMouse()` ile açılır ve
  yalnızca terminal fare olayını taşıyorsa çalışır (Windows Terminal evet,
  eski conhost hayır). Fare yoksa klavye yolu BİREBİR aynı ve eksiksiz
  kalır; seçim göstergesi giriş yolundan bağımsızdır (iki yol aynı `►`).

## 4. Klavye

Görünür sözlük: `↑↓` seç · `Enter` çalıştır · `Tab` panel · `Esc` kapat ·
`?` yardım · `q` çık. Alt bilgi odaya göre değişir (İşlemler odaktayken
seçim ipucu, Kayıtlar odaktayken kaydırma ipucu).
Öneri: harf kısayolları eylemlerden TAMAMEN kalkar (tek cümlelik gerekçe:
keşfedilebilir değiller ve kayıt odasındaki `j/k/g/G` ile çakışıyorlar;
ok + fare her şeyi karşılıyor). `j/k` yalnızca Kayıtlar odasında kaydırma
takma adı olarak gizli kalır.

## 5. Görsel dil

- Renk yalnızca anlam taşır: kayıt düzeyi (INFO/WARN/ERROR) + durum rozeti
  (`● Çalışıyor` / `○ Durdu`). Krom tek renktir; açık uçlu terminalde de
  okunur (koyu-zemin varsayımı yok, mutlak palet adı yok).
- Seçim = ters video + `►`; odak = çift kenarlık + `►`; hover = `›` + kalın.
- `NO_COLOR` tanımlıysa tüm ANSI kapanır; işaretler ve kenarlıklar aynen
  kalır (kutu çizgileri renk değildir).

## 6. Tepsi (seçenekler ve ödünler)

Gerçeklik: TUI bazen panele istemci bağlanır; tepsi ayrı bir süreçtir.
- (a) Tepsi sürecini başlat/durdur: yapılabilir (`orpanel --tray` + sahiplik
  denetimli durdurma). Artı: istek karşılanır. Eksi: istemci bağlıyken
  sunucuyu durdurmak uyarı ister.
- (b) Tepsi simgesini göster/gizle: dışarıdan MÜMKÜN DEĞİL; simgeyi yalnızca
  tepsi süreci yönetir, yeni API/protokol ister.
- (c) Tepsi menüsünü TUI'dan açmak: Windows'ta MÜMKÜN DEĞİL; menü tepsi
  sürecinin mesaj döngüsüne aittir, başka süreçten çağrılamaz. Dürüst cevap.
- (d) Yalnızca tepsi ayarları (otomatik başlat): zaten var, en ucuz ama
  isteği karşılamaz.
Öneri: (a) + (d) — Durum odasında `tepsi: açık/kapalı` satırı + İşlemler'de
`Tepsi` aç/kapa eylemi. Sahibine sorulacak: tepsi denetimi mi istiyordu?

## 7. Emin olmadıklarım (uydurmadım)

- Tepsi isteğinin gerçek niyeti (a) mı yoksa (c) mi?
- Sahibinin terminalinde fare olayları geliyor mu?
- `Kur` onaysız mı kalmalı (şu an öyle)?
- Açık uçlu terminalde renk doğrulaması yapılmadı.
- tview fare API'sinin tam adı uygulamada doğrulanacak.

## 8. Mockup'lar (verbatim, kutu genişlikleri denetlendi)

Lejant: `►` seçim (ters video), `›` fare-hover (kalın), `*` onay ister,
`★` güncelleme mevcut. Renk mockup'ta görünmez.

### 120x30 · çalışıyor

### 120x30 · çalışıyor
```
┌──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│ OrPanel v1.4.5  OmniRoute 3.8.49  tr/koyu  12:00:00                                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
╔┤ ► Durum ├═══════════════════════════════╗┌┤ Kayıtlar ▲ 5/5 ├────────────────────────────────────────────────────────┐
║Durum      ● Çalışıyor                    ║│[12:00:01] INFO: panel başladı                                            │
║Versiyon   3.8.49                         ║│[12:00:02] INFO: sağlık sondası yanıt verdi (200)                         │
║Port       20128                          ║│[12:00:03] WARN: kurtarma beklemesi aktif (3 deneme)                      │
║Node       24.20.0                        ║│[12:00:04] ERROR: sürüm sorgusu zaman aşımı, önbellek kullanıldı          │
║tepsi      açık                           ║│[12:00:05] INFO: yapılandırma kaydedildi (dil=tr)                         │
║yönetim    panel (:20127)                 ║│                                                                          │
║                                          ║│                                                                          │
║                                          ║│                                                                          │
╚══════════════════════════════════════════╝│                                                                          │
╔┤ ► İşlemler (8) ├════════════════════════╗│                                                                          │
║► Durdur *                                ║│                                                                          │
║  Yeniden Başlat *                        ║│                                                                          │
║  ★ Güncelle *                            ║│                                                                          │
║  Onar *                                  ║│                                                                          │
║  Web Arayüzü                             ║│                                                                          │
║  Otomatik Başlat: açık                   ║│                                                                          │
║  Dil: Türkçe                             ║│                                                                          │
║  Tema: koyu                              ║│                                                                          │
╚══════════════════════════════════════════╝└──────────────────────────────────────────────────────────────────────────┘
 ↑↓ seç · Enter çalıştır · Tab panel · ? yardım · q çık                                                                 
```

### 120x30 · durdu
```
┌──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│ OrPanel v1.4.5  OmniRoute 3.8.49  tr/koyu  12:00:00                                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
╔┤ ► Durum ├═══════════════════════════════╗┌┤ Kayıtlar ▲ 3/3 ├────────────────────────────────────────────────────────┐
║Durum      ○ Durdu                        ║│[12:00:01] INFO: panel başladı                                            │
║Versiyon   3.8.49                         ║│[12:00:02] WARN: omniroute yanıt vermiyor                                 │
║Port       20128                          ║│[12:00:03] INFO: yeniden deneme zamanlandı                                │
║Node       24.20.0                        ║│                                                                          │
║tepsi      kapalı                         ║│                                                                          │
║yönetim    bu süreç                       ║│                                                                          │
║                                          ║│                                                                          │
║                                          ║│                                                                          │
╚══════════════════════════════════════════╝│                                                                          │
╔┤ ► İşlemler (7) ├════════════════════════╗│                                                                          │
║► Başlat                                  ║│                                                                          │
║  ★ Güncelle *                            ║│                                                                          │
║  Onar *                                  ║│                                                                          │
║  Web Arayüzü                             ║│                                                                          │
║  Otomatik Başlat: kapalı                 ║│                                                                          │
║  Dil: Türkçe                             ║│                                                                          │
║  Tema: koyu                              ║│                                                                          │
╚══════════════════════════════════════════╝└──────────────────────────────────────────────────────────────────────────┘
 ↑↓ seç · Enter çalıştır · Tab panel · ? yardım · q çık                                                                 
```

### 120x30 · fare-hover (seçim Yeniden Başlat'ta, hover Güncelle'de)
```
┌──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│ OrPanel v1.4.5  OmniRoute 3.8.49  tr/koyu  12:00:00                                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
╔┤ ► Durum ├═══════════════════════════════╗┌┤ Kayıtlar ▲ 2/2 ├────────────────────────────────────────────────────────┐
║Durum      ● Çalışıyor                    ║│[12:00:01] INFO: panel başladı                                            │
║Versiyon   3.8.49                         ║│[12:00:02] INFO: sağlık sondası yanıt verdi (200)                         │
║Port       20128                          ║│                                                                          │
║Node       24.20.0                        ║│                                                                          │
║tepsi      açık                           ║│                                                                          │
║yönetim    panel (:20127)                 ║│                                                                          │
║                                          ║│                                                                          │
║                                          ║│                                                                          │
╚══════════════════════════════════════════╝│                                                                          │
╔┤ ► İşlemler (8) ├════════════════════════╗│                                                                          │
║  Durdur *                                ║│                                                                          │
║► Yeniden Başlat *                        ║│                                                                          │
║› ★ Güncelle *                            ║│                                                                          │
║  Onar *                                  ║│                                                                          │
║  Web Arayüzü                             ║│                                                                          │
║  Otomatik Başlat: açık                   ║│                                                                          │
║  Dil: Türkçe                             ║│                                                                          │
║  Tema: koyu                              ║│                                                                          │
╚══════════════════════════════════════════╝└──────────────────────────────────────────────────────────────────────────┘
 ↑↓ seç · Enter çalıştır · Tab panel · ? yardım · q çık                                                                 
```

### 80x24 · çalışıyor (odak İşlemler'de, liste taşıyor)
```
┌──────────────────────────────────────────────────────────────────────────────┐
│ OrPanel v1.4.5  tr/koyu  12:00:00                                            │
└──────────────────────────────────────────────────────────────────────────────┘
┌┤ Durum ├─────────────────────────────────────────────────────────────────────┐
│Durum      ● Çalışıyor                                                        │
│Versiyon   3.8.49                                                             │
│Port       20128                                                              │
│Node       24.20.0                                                            │
│tepsi      açık                                                               │
│yönetim    panel (:20127)                                                     │
└──────────────────────────────────────────────────────────────────────────────┘
╔┤ ► İşlemler (6) ├════════════════════════════════════════════════════════════╗
║► Durdur *                                                                    ║
║  Yeniden Başlat *                                                            ║
║  ★ Güncelle *                                                                ║
║  Onar *                                                                      ║
║  Web Arayüzü                                                                 ║
║  … +3 daha ↓                                                                 ║
╚══════════════════════════════════════════════════════════════════════════════╝
┌┤ Kayıtlar ▲ 5/5 ├────────────────────────────────────────────────────────────┐
│[12:00:04] ERROR: sürüm sorgusu zaman aşımı                                   │
│[12:00:05] INFO: yapılandırma kaydedildi                                      │
└──────────────────────────────────────────────────────────────────────────────┘
 ↑↓ seç · Enter çalıştır · Tab panel · ? yardım · q çık                         
```

### 80x24 · durdu (boş kayıt odası)
```
┌──────────────────────────────────────────────────────────────────────────────┐
│ OrPanel v1.4.5  tr/koyu  12:00:00                                            │
└──────────────────────────────────────────────────────────────────────────────┘
┌┤ Durum ├─────────────────────────────────────────────────────────────────────┐
│Durum      ○ Durdu                                                            │
│Versiyon   3.8.49                                                             │
│Port       20128                                                              │
│Node       24.20.0                                                            │
│tepsi      kapalı                                                             │
│yönetim    bu süreç                                                           │
└──────────────────────────────────────────────────────────────────────────────┘
╔┤ ► İşlemler (5) ├════════════════════════════════════════════════════════════╗
║► Başlat                                                                      ║
║  ★ Güncelle *                                                                ║
║  Onar *                                                                      ║
║  Web Arayüzü                                                                 ║
║  … +3 daha ↓                                                                 ║
╚══════════════════════════════════════════════════════════════════════════════╝
┌┤ Kayıtlar ├──────────────────────────────────────────────────────────────────┐
│(kayıt yok — panel yeni başladı)                                              │
└──────────────────────────────────────────────────────────────────────────────┘
 ↑↓ seç · Enter çalıştır · Tab panel · ? yardım · q çık                         
```

### 80x24 · yardım penceresi
```
┌──────────────────────────────────────────────────────────────────────────────┐
│ OrPanel v1.4.5  tr/koyu  12:00:00                                            │
└──────────────────────────────────────────────────────────────────────────────┘
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░╔══════════════════════════════════════════════════════╗░░░░░░░░░░░░
░░░░░░░░░░░░║Yardım                                                ║░░░░░░░░░░░░
░░░░░░░░░░░░║GEZİNME                                               ║░░░░░░░░░░░░
░░░░░░░░░░░░║↑↓/Tab  panel ve seçim                                ║░░░░░░░░░░░░
░░░░░░░░░░░░║Enter    çalıştır / onayla                            ║░░░░░░░░░░░░
░░░░░░░░░░░░║Esc      kapat                                        ║░░░░░░░░░░░░
░░░░░░░░░░░░║                                                      ║░░░░░░░░░░░░
░░░░░░░░░░░░║İŞLEMLER (durum: çalışıyor)                           ║░░░░░░░░░░░░
░░░░░░░░░░░░║Durdur *                                              ║░░░░░░░░░░░░
░░░░░░░░░░░░║Yeniden Başlat *                                      ║░░░░░░░░░░░░
░░░░░░░░░░░░║★ Güncelle *                                          ║░░░░░░░░░░░░
░░░░░░░░░░░░║Onar *                                                ║░░░░░░░░░░░░
░░░░░░░░░░░░║Web Arayüzü                                           ║░░░░░░░░░░░░
░░░░░░░░░░░░║(* onay ister)                                        ║░░░░░░░░░░░░
░░░░░░░░░░░░╚══════════════════════════════════════════════════════╝░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
 Esc kapat · q çık                                                              
```

### 80x24 · onay penceresi
```
┌──────────────────────────────────────────────────────────────────────────────┐
│ OrPanel v1.4.5  tr/koyu  12:00:00                                            │
└──────────────────────────────────────────────────────────────────────────────┘
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░╔══════════════════════════════════════════╗░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░║Yeniden Başlat?                           ║░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░║                                          ║░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░║Bu işlem OmniRoute'u yeniden              ║░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░║başlatır. Devam edilsin mi?               ║░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░║                                          ║░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░║[ onayla ]   [ vazgeç ]                   ║░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░╚══════════════════════════════════════════╝░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
 Enter onayla · Esc vazgeç                                                      
```

## 9. Onay için sorular (sahibine)

1. Tepsi denetimi mi istiyordu (a), yoksa tepsi menüsünü mü (c — mümkün değil)?
2. Terminalinde fare olayları geliyor mu?
3. `Kur` onaysız kalsın mı?
