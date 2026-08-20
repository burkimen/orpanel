# Omniroute Panel - Proje Bilgilendirme Dokümanı

## 📋 Genel Bakış
**Omniroute Panel**, Omniroute ekosistemindeki servislerin durumunu anlık olarak denetleyen, arka planda (sistem tepsisinde) sessizce çalışan ve kullanıcıya modern bir web arayüzü sunan Go tabanlı çapraz platform masaüstü yardımcı aracıdır[cite: 18].

## 🛠️ Mimari ve Teknik Altyapı
* **Çapraz Platform ve Evrensel Yapı**: Uygulama artık hedef sistemde harici taşınabilir Node.js bağımlılıkları barındırmaz[cite: 18]. Doğrudan sistem genelinde kurulu olan standart Node.js motorunu ve küresel `npm` modül dizinlerini (`omniroute.mjs`) baz alır[cite: 18].
* **Güvenilirlik ve Watchdog**: Servislerin sağlığını sürekli denetleyen, beklenmeyen durumlarda otomatik olarak yeniden ayağa kaldıran dahili bir watchdog mekanizmasına sahiptir[cite: 18].
* **Teknoloji Yığını**: 
  * **Backend**: Go (Go-routine tabanlı süreç yönetimi, HTTP API sunucusu, yerel kayıt defteri/sistem entegrasyonları).
  * **Frontend**: HTML5, Xterm.js (terminal akışı), Google Material Symbols, Material Design 3 (MD3) tasarım ilkeleri[cite: 18].

## 🚀 Derleme ve Dağıtım (Build)
Projenin her işletim sisteminde kolayca derlenebilmesi için kök dizine özel betikler eklenmiştir[cite: 18]:
* **Windows**: `build.bat` betiği ile grafik arayüz bayraklı (`-ldflags "-H=windowsgui"`) derleme[cite: 18].
* **Linux / macOS**: `build.sh` betiği ile standart ikili dosya (binary) üretimi[cite: 18].

## 🎨 Modüler Tema Sistemi ve Hedefler
* **Harici Stil Yönetimi**: `panel.go` içerisindeki tüm statik CSS ve renk paletleri ayrı bir tema dizinine taşınarak kod tabanı sadeleştirilecektir.
* **Üçlü Tema Yönetimi**: Web arayüzünün Ayarlar sekmesinde **Açık**, **Koyu** ve **Sistem** olmak üzere 3 seçenek sunulacaktır. İlk kurulumda/config yokken "Sistem" aktif olacak, tarayıcının renk tercihine göre (`prefers-color-scheme`) dinamik uyum sağlanırken arayüz ayarı "Sistem" olarak kalmaya devam edecektir.