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
| kurulu değil | Kur * | Web Arayüzü, Dil, Tema | Kur |
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
- Onay kuralı (§9b): Kur, Onar, Güncelle, Durdur, Yeniden Başlat VE çıkış
  (`q`) her zaman onay ister. `q` onayı panel/tepsi çalışmaya devam eder
  der ve güvenli varsayılan `vazgeç`'tir.

## 3. Fare modeli

- Hover satırı `›` işareti + kalın gösterir; seçim `►` + ters video.
  İkisi aynı anda görünebilir (seçim Yeniden Başlat'ta, hover Güncelle'de).
- Tık seçer; yıkıcı olmayanda ikinci tık / çift tık çalıştırır, yıkıcıda
  onay penceresi açar. Tekerlek Kayıtlar/Yardım'ı kaydırır. Panele tıklamak
  odağı taşır. Modal düğmeleri tıklanabilir; varsayılan `onayla` (Enter).
- KUSUR (v1.4.5'te doğrulandı): fare hiç açılmıyor — uygulama
  `EnableMouse(true)` çağırmıyor, bu yüzden tcell terminalin fare
  protokolünü istemiyor ve hiçbir fare olayı gelmiyor. Düzeltme: tview
  uygulamasında `EnableMouse(true)` + hover/tık/tekerlek/tıklanabilir
  modal düğmeleri (varsayılan `onayla`).
- Sonuçlar (dürüst): (a) fare olayı taşımayan uçta klavye yolu BİREBİR
  aynı ve eksiksiz kalır (iki yol aynı `►` seçim göstergesi); (b) fare
  kipi açıkken uçbirimde metin seçimi genelde Shift gerektirir — yardım
  veya README bunu yazar.
- Doğrulama: koşum fare enjekte edemez; en yakın kanıt, aynı işleyiciye
  sentetik fare olayı sürmek + sahibinin terminalinde gerçek yol onayıdır.

## 4. Klavye

Görünür sözlük: `↑↓` seç · `Enter` çalıştır · `Tab` panel · `Esc` kapat ·
`?` yardım · `q` çık. Alt bilgi odaya göre değişir (İşlemler odaktayken
seçim ipucu, Kayıtlar odaktayken kaydırma ipucu).
Karar (sahip onayı): `j/k` TAMAMEN kalkar — gizli takma ad bile yok,
kaydırma odasında bile yok. Gerekçe: keşfedilemezler ve ok+fare her şeyi
karşılıyor. Sözlük: oklar + Tab + Enter + Esc + `?` + `q` (hepsi görünür).

## 5. Görsel dil

- Anlam renkleri: kayıt düzeyi (INFO/WARN/ERROR) + durum rozeti
  (`● Çalışıyor` / `○ Durdu`).
- TUI paleti gerçektir (sahip onayı: `t` bugüne dek yalnız web arayüzünü
  boyuyordu): `koyu` kendi arka/ön-plan/kenarlık/seçim renklerini çizer,
  `açık` açık uçlu uçbirimde okunur eşdeğerini çizer, `sistem` bilerek
  uçbirimin kendi varsayılanlarını kullanır (arka planı tahmin etmeyiz —
  ödün: sistemde marka renkleri yok, ama hiçbir uçta okunmazlık yok).
- Seçim = ters video + `►`; odak = çift kenarlık + `►`; hover = `›` + kalın.
- `NO_COLOR` tanımlıysa her şeyin üstüne yazar: ANSI kapanır, işaretler ve
  kenarlıklar aynen kalır. `t` eylemi değişen paleti mesaj satırında söyler.
- Mockup'lar (§12): aynı karenin `koyu` ve `açık` çizimleri yan yanadır
  (ters-video/kalın metinde gösterilemez, fark başlıkta yazar).

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
Karar: §8'e taşındı (tepsiyi durdurmak sunulmuyor; yalnızca rapor + otomatik başlat).

## 8. Tepsi/panel önyükleme (argümansız açılış kusuru)

Kusur: `orpanel` (argümansız) TUI açar ama tepsi/panel süreci başlatmaz;
kullanıcı çıkıp ayrıca `orpanel --tray` koşmak zorunda. Tasarım: argümansız
açılışta önce 127.0.0.1:20127 yoklanır (kısa zaman aşımı, mevcut panel
yoklama yardımcısı). Yanıt varsa ona istemci bağlanılır. Yanıt yoksa aynı
ikili `--tray` ile ayrık başlatılır (`spawnDetached`: DETACHED_PROCESS |
CREATE_NEW_PROCESS_GROUP, pencere gizli — `--tray` yolunun aynısı), sonra
TUI istemci olarak açılır. Böylece simge ilk kareden vardır.
- Durum odası her zaman yazar: `yönetim: panel (:20127)` (istemci) ya da
  `yönetim: bu süreç` (doğrudan). Belirsiz durum yok.
- Tepsiyi TUI içinden DURDURMAK sunulmaz: tepsiyi durdurmak bekçi
  sürecini (watchdog) da öldürür; bu, paneli bilinçli olarak başsız
  bırakmaktır. Tasarımda İşlemler'de `Tepsi` aç/kapa eylemi YOKTUR.
  Öneri değişti (§6'daki (a) geri çekildi): tepsi yalnızca Durum satırında
  `tepsi: açık/kapalı` diye raporlanır + `Otomatik Başlat` ayarı kalır.
  Gerekçe: kazara tıklanan bir `Tepsi'yi durdur`, gözetimsiz panel + ölü
  bekçi bırakır; onayı bile olsa maliyeti faydasını aşar.

## 9. Menü gruplama (eylem listesi)

Sahibi kritik/yıkıcı işlemlerin karışık listede durmasını istemiyor.
Üst düzey İşlemler yalnızca durum eylemlerini + iki grup girişini taşır
(çalışıyor durumunda: Durdur, Yeniden Başlat, ▸ Bakım, ▸ Ayarlar).
- `▸ Bakım` bir alt liste/modal açar: Kur, Onar, Güncelle (üçü de onaylı).
- `▸ Ayarlar` bir alt liste/modal açar: Otomatik Başlat (aç/kapa),
  Dil (döngü), Tema (döngü), Web Arayüzü.
- Davranış: grup satırında Enter/fare-tık grubu açar; grup içinde Esc bir
  üst düzeye döner (uygulamadan çıkmaz). Fare tekerleği grup listesinde de
  kaydırır. Yardım ekranı grupları açık yazar (örn. `Bakım ▸ Onar *`).
- Mockup'lar (§12) güncellendi: işlem odası grup başlıklı dikey listedir.

## 9b. Çıkış onayı (sahip kararı: `q` anında çıkmaz)

`q` her zaman onay penceresi açar (Kur/Onar/Güncelle/Durdur/Yeniden Başlat
ile aynı `*` kapısı). Metin: "TUI kapatılsın mı? Panel ve tepsi arka planda
çalışmaya devam eder." Düğmeler: `[ kapat ]  [ vazgeç ]`, güvenli varsayılan
`vazgeç` (Enter `vazgeç`'te durur; `kapat` için Tab+Enter gerekir).
Esc = vazgeç. Gerekçe: tepsi/panel ayrı süreçtir; TUI'yi kapatmak hizmeti
durdurmaz — bunu söylemeyen çıkış, "kapattım ama simge hâlâ orada" şaşkınlığı
yaratır. Yardım bu davranışı açık yazar.

## 10. İstemci modunda kayıt odası (boş kutu kusuru)

Kusur: TUI kendi sürecindeki `logBuffer`'ı çizer; panele istemci bağlıyken
kayıtlar sunucu süreçtedir, bu yüzden `Kayıtlar` yapısal olarak boş görünür.
Tasarım: istemci modunda kayıt odası panelin mevcut uç noktasını yoklar:
`GET /api/logs?last=N` → `{logs: [...], newIndex: M}` (panel.go'daki
mevcut imza; `last` kaçırılan ön-ek dizinidir, `newIndex` toplam uzunluk).
- Yoklama artan `last=newIndex` ile yapılır (tamponu baştan çekmek yok);
  doğrudan modda yalnızca süreç-içi tampon kullanılır.
- Oda her zaman açık durum yazar: `yükleniyor…` (ilk yoklama öncesi),
  `kayıt yok — panel yeni başladı` (boş yanıt), asla boş kutu yok.
- Başlık sayacı sunucu dizininden gelir (`Kayıtlar ▲ newIndex-son`).

## 11. Emin olmadıklarım (uydurmadım)

- Açık uçlu terminalde renk doğrulaması yapılmadı (`açık` palet göz onayı
  bekliyor).
- Fare gerçek yolu yalnızca sahibinin terminalinde doğrulanabilir (koşum
  sentetik olay verir; §3'teki kanıt notuna bak).
- `sistem` paletinde marka renklerinden vazgeçmek kabul mü?

## 12. Mockup'lar (verbatim, kutu genişlikleri denetlendi)

Lejant: `►` seçim (ters video), `›` fare-hover (kalın), `*` onay ister (çıkış dahil),
`★` güncelleme mevcut. Renk mockup'ta görünmez.

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


### 120x30 · gruplu işlemler (çalışıyor) + istemci kayıt başlığı
```
┌──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│ OrPanel v1.4.5  OmniRoute 3.8.49  tr/koyu  12:00:00                                                                  │
└──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
╔┤ ► Durum ├═══════════════════════════════╗┌┤ Kayıtlar ▲ 3/3 (panel) ├────────────────────────────────────────────────┐
║Durum      ● Çalışıyor                    ║│[12:00:01] INFO: panel başladı                                            │
║Versiyon   3.8.49                         ║│[12:00:02] INFO: sağlık sondası yanıt verdi (200)                         │
║Port       20128                          ║│[12:00:03] WARN: kurtarma beklemesi aktif (3 deneme)                      │
║Node       24.20.0                        ║│                                                                          │
║tepsi      açık                           ║│                                                                          │
║yönetim    panel (:20127)                 ║│                                                                          │
║                                          ║│                                                                          │
║                                          ║│                                                                          │
╚══════════════════════════════════════════╝│                                                                          │
╔┤ ► İşlemler (4) ├════════════════════════╗│                                                                          │
║► Durdur *                                ║│                                                                          │
║  Yeniden Başlat *                        ║│                                                                          │
║  ▸ Bakım …                               ║│                                                                          │
║  ▸ Ayarlar …                             ║│                                                                          │
╚══════════════════════════════════════════╝└──────────────────────────────────────────────────────────────────────────┘
 ↑↓ seç · Enter aç/çalıştır · Tab panel · ? yardım · q çık                                                              
```

### 80x24 · Bakım alt grubu (Enter/⊕ tık ile açılır, Esc geri döner)
```
┌──────────────────────────────────────────────────────────────────────────────┐
│ OrPanel v1.4.5  tr/koyu  12:00:00                                            │
└──────────────────────────────────────────────────────────────────────────────┘
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░╔══════════════════════════════════════╗░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░║Bakım                                 ║░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░║► ★ Güncelle *                        ║░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░║  Onar *                              ║░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░║  Kur *                                ║░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░║  (* onay ister)                      ║░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░╚══════════════════════════════════════╝░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
 ↑↓ seç · Enter çalıştır · Esc geri                                             
```
### 80x24 · çıkış onayı (sahip kararı: `q` her zaman sorar)
```
┌──────────────────────────────────────────────────────────────────────────────┐
│ OrPanel v1.4.5  tr/açık  12:00:00                                            │
└──────────────────────────────────────────────────────────────────────────────┘
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░╔══════════════════════════════════════════╗░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░║Kapatılsın mı?                            ║░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░║                                          ║░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░║TUI kapanır; panel ve tepsi               ║░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░║arka planda çalışmaya                     ║░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░║devam eder.                               ║░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░║                                          ║░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░║[ kapat ]   [ vazgeç ]                    ║░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░╚══════════════════════════════════════════╝░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
 Tab seç · Enter onayla (vazgeç varsayılan) · Esc vazgeç                        
```

### 80x24 · palet ikizi (aynı kare: `koyu` / `açık` — fark başlıkta, ters-video metinde gösterilemez)
koyu:
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
açık:
```
┌──────────────────────────────────────────────────────────────────────────────┐
│ OrPanel v1.4.5  tr/açık  12:00:00                                            │
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

## 13. Onay için sorular (sahibine)

1. Tepsi denetimi mi istiyordu (a), yoksa tepsi menüsünü mü (c — mümkün değil)?
2. Terminalinde fare olayları geliyor mu? (v1.4.5'te fare kapalıydı — §3)
3. ~~`Kur` onaysız kalsın mı?~~ → karar: `Kur *` (onaylı) + `q` onaylı (§9b).
