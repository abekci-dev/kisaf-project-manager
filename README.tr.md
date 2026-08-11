<div align="center">

# kisaf

### keep it simple as fuck project manager

**Bilgisayarındaki proje klasörlerini tek yerden yönet.**
Git durumunu gör, Explorer'da göster, VS Code veya JetBrains'te aç.

Tek dosya · sıfır bağımlılık · ~9 MB · kurulum gerektirmez

[English](README.md) · [GPL-3.0](LICENSE)

</div>

> **kisaf** = *keep it simple as fuck*. Programın adı ve sloganı özel ad
> olduğu için hiçbir dile çevrilmiyor.

---

## Ne işe yarar

Diskte biriken onlarca proje klasörünün içinde kaybolmayı bitirir. kisaf,
makinende çalışan küçük bir sunucu ve web arayüzünden ibaret.

- **Kart görünümü** — her proje bir kart: adı, açıklaması, git durumu,
  etiketleri ve açık görev sayısı tek bakışta. Kartın üzerindeki düğmelerle
  projeyi açmadan editörde/Explorer'da/terminalde açabilirsin. Çok proje varsa
  **sıkışık liste** moduna geçersin.
- **Görevler** — her projeye kutucuklu görev ekle, tamamla, sil. Öncelik seç
  (düşük/normal/yüksek), sırala, tamamlananları tek tıkla temizle. Kartta
  ilerleme çubuğu olarak görünür.
- **Proje ekle** — klasörü arayüzden seç, listeye eklensin.
- **Klasör tara** — `D:\Projeler` göster, altındaki bütün git depolarını bulup
  topluca eklesin.
- **Git durumu** — dal adı, kaç değişiklik var, kaç commit geride/ileride.
- **Commit geçmişi** — commit açıklamalarını arayüzde oku, tıklayınca gövdesi
  açılsın.
- **Explorer'da göster** — proje klasörü Gezgin'de seçili olarak açılsın.
- **Editörde aç** — VS Code, Cursor, Windsurf, Zed, Sublime, IntelliJ, Rider,
  PyCharm… otomatik bulunur; her proje için ayrı editör seçilebilir.
- **Terminal** — Windows Terminal / PowerShell doğrudan o klasörde açılsın.
- **Notlar ve etiketler** — serbest not, `iş` / `arşiv` gibi etiketler.
- **Arama** — proje adı, yol, etiket, not *ve görev metinleri* içinde arar.
- **README önizleme** ve **dosya ağacı** — projeye girmeden içine bakabilme.

Diskteki hiçbir dosyaya dokunmaz. Sadece klasörlere *işaret eder*.

### Görevler nasıl çalışıyor

Görev listesi, "bu projede nerede kalmıştım" sorusunun serbest not yerine
işaretlenebilir bir cevabı. Her görevin metni, tamamlanma durumu ve önceliği
var:

| İşlem | Nasıl |
|---|---|
| Ekle | Üstteki kutuya yaz, önceliği seç, Enter |
| Tamamla / geri al | Baştaki kutucuk |
| Metni düzelt | Metne tıkla, yerinde düzenlenir (Enter kaydeder, Esc vazgeçer) |
| Öncelik değiştir | Öncelik etiketine tıkla (normal → yüksek → düşük) |
| Sırala | Satırdaki ↑ ↓ |
| Sil | Satırdaki ✕ |
| Toplu temizlik | "Tamamlananları temizle" |

Açık görevi olan projeleri **◧ Açık görev** filtresiyle süzebilir, yüksek
öncelikli açık görevleri kartta kırmızı **acil** rozetinden görebilirsin.

---

## Diller

Arayüz **Türkçe** ve **İngilizce** olarak geliyor. Varsayılan olarak tarayıcının
dilini izler; Ayarlar → Dil'den sabitleyebilirsin.

Yeni dil eklemek tek dosyalık iş: [`web/i18n.js`](web/i18n.js) içindeki `en`
bloğunu kopyala, değerleri çevir, anahtarları koru ve dil kodunu `LANGUAGES`
listesine ekle. Sunucu hataları da çevriliyor — API sabit bir kod ve
değişkenlerini gönderiyor, cümleyi arayüz kuruyor.

---

## Kurulum

### Hazır dosyayla (önerilen)

1. [Releases](../../releases) sayfasından `kisaf.exe` dosyasını indir
2. Çift tıkla — tarayıcı kendiliğinden açılır

Kalıcı kurulum (masaüstü kısayolu, Windows açılışında otomatik başlatma, `kisaf`
adı):

```powershell
.\scripts\install-windows.ps1
```

Ağdan (telefon, başka bilgisayar) erişecekseniz:

```powershell
# Yönetici PowerShell'inde
.\scripts\install-windows.ps1 -AllowNetwork
```

Kaldırmak için `.\scripts\install-windows.ps1 -Uninstall` — proje listeniz
silinmez.

### Kaynaktan derleme

Yalnızca [Go 1.24+](https://go.dev/dl/) gerekir. Başka hiçbir şey.

```powershell
go build -o kisaf.exe .          # veya:  .\scripts\build.ps1
```

```bash
go build -o kisaf .              # Linux / macOS
```

### Yeni sürüme geçmek

Yeni `kisaf.exe`'yi çalıştırmanız yeterli. Eski bir sürüm o an çalışıyorsa
kendini kapatır, yeni sürüm portu devralır ve tarayıcı güncel arayüzü gösterir.
Eski sürüm süreçten düştüğü için `kisaf.exe` dosyası da artık silinebilir hâle
gelir.

Aynı sürüm zaten çalışıyorsa ikinci bir sunucu açılmaz; sadece tarayıcı sekmesi
gelir.

---

## Adresler: "localhost yazmadan"

Program açıldığında üç yoldan da erişilebilir:

| Adres | Nereden çalışır | Nasıl |
|---|---|---|
| `http://kisaf` | bu bilgisayar (Windows) | LLMNR — Windows'un yerleşik ad çözümlemesi |
| `http://kisaf.local` | aynı ağdaki her cihaz | mDNS/Bonjour — Windows 10+, macOS, iOS, Android |
| `http://localhost` | bu bilgisayar | her zaman çalışır, yedek adres |

Port 80 kullanılıyorsa adreste port yazmanıza gerek kalmaz. Port meşgulse
program otomatik olarak 7777'ye düşer ve adresi konsola/günlüğe yazar.

**Uygulama gibi kullanmak için:** tarayıcıda adres çubuğundaki *Yükle* simgesine
basın. kisaf bir PWA olarak kurulur; kendi penceresinde, adres çubuğu olmadan
açılır. Ayrıca sistem tepsisinde bir simge durur — tıklayınca arayüz açılır,
sağ tıklayınca menü gelir.

---

## Uzaktan erişim (homelab)

Varsayılan olarak **yalnızca bu bilgisayardan** erişilebilir. Başka bir
makineden gelen istekler, sebebi açıklanarak reddedilir.

Açmak için `%APPDATA%\kisaf\config.json` dosyasındaki `token` alanına bir parola
yazın ve programı yeniden başlatın:

```json
{
  "host": "kisaf",
  "port": 80,
  "bind": "0.0.0.0",
  "token": "buraya-uzun-bir-parola",
  "allowedHosts": ["projeler.evim.lan"]
}
```

Artık ağdan gelen ziyaretçiler bir giriş ekranıyla karşılaşır. Aynı binary'yi
homelab sunucunuza kopyalayıp systemd/Docker altında da çalıştırabilirsiniz —
`KISAF_TOKEN`, `KISAF_PORT`, `KISAF_BIND`, `KISAF_HOST`, `KISAF_DATA_DIR`
ortam değişkenleri config dosyasını ezer.

> Ters vekil sunucu (nginx/Caddy) arkasına koyacaksanız, kullandığınız alan adını
> `allowedHosts` listesine ekleyin — aksi hâlde istek reddedilir.

---

## Güvenlik

Bu program **sizin adınıza program çalıştırabildiği** için, rastgele bir web
sayfasının onu kullanamaması gerekir. Üç katman bunu engeller:

1. **Host beyaz listesi** — DNS rebinding saldırısını keser. Saldırganın alan
   adı `127.0.0.1`'e çözülse bile istek `Host` başlığında takılır.
2. **Origin denetimi** — değişiklik yapan her istekte kaynak kontrol edilir;
   başka bir sekmedeki sayfa CSRF ile proje ekleyemez/silemez.
3. **Anahtar** — bu bilgisayar dışından gelen her istek için zorunlu.

Ayrıca:

- API'ye hiçbir zaman "şu komutu çalıştır" denmez. Yalnızca bir **editör
  kimliği** gönderilir; o kimlik, tespit edilmiş programlar listesinde aranır.
- Dosya okuma uçları proje klasörünün dışına çıkamaz — sembolik bağlantılar
  çözülerek kontrol edilir.

---

## Kısayollar

| Tuş | İş |
|---|---|
| `/` | aramaya odaklan |
| `Enter` (arama kutusunda) | ilk sonucu aç |
| `Ctrl` + `N` | yeni proje |
| `Esc` | aramayı temizle / listeye dön / pencereyi kapat |
| `Enter` (kart seçiliyken) | projeyi aç |

---

## Dosyalar

| Yol | İçerik |
|---|---|
| `%APPDATA%\kisaf\data.json` | proje listeniz — düz JSON, elle düzenlenebilir, yedeklenebilir |
| `%APPDATA%\kisaf\config.json` | port, ad, anahtar gibi süreç ayarları |
| `%APPDATA%\kisaf\kisaf.log` | günlük (bir şey ters giderse buraya bakın) |

Linux/macOS'ta bu klasör `~/.config/kisaf` altındadır.

---

## Komut satırı

```
kisaf [seçenekler]

  --port <n>       dinlenecek port (config.json'u ezer)
  --no-tray        sistem tepsisi simgesini başlatma
  --no-browser     açılışta tarayıcı açma
  --no-mdns        ağ keşfini (.local adı) kapat
  --version        sürümü yazdır ve çık
```

---

## Nasıl çalışıyor

```
kisaf.exe  ─┬─ HTTP sunucusu ── gömülü web arayüzü (embed.FS)
            │                   REST API
            ├─ mDNS + LLMNR ─── kisaf.local / kisaf adlarını yanıtlar
            ├─ tepsi simgesi ── Win32 (syscall, cgo yok)
            ├─ simgeler ─────── çalışma anında çizilir (internal/icon)
            └─ git ──────────── sistemdeki git komutunu çağırır
```

Tasarım kararları:

- **Neden yerel sunucu, neden saf web sayfası değil?** Tarayıcıdaki bir sayfa
  Explorer'ı açamaz, VS Code'u başlatamaz, diskteki klasörleri gezemez. Bu işler
  için makinede çalışan bir süreç şart. Aynı süreç, uzaktan erişim kapısını da
  bedavaya açık bırakıyor.
- **Neden Go, tek binary?** Kurulum yok, runtime yok, `node_modules` yok. Dosyayı
  kopyala, çalıştır. Homelab'e taşımak da tek dosya kopyalamak demek.
- **Neden JSON, veritabanı değil?** Birkaç yüz kayıt için SQLite sürücüsü taşımak
  gereksiz. JSON okunabilir, yedeklenebilir, elle düzeltilebilir.
- **Neden `git` komutu, kütüphane değil?** Kodun onda biri, ve gösterdiği şey
  kendi terminalinizde gördüğünüzle her zaman birebir aynı.
- **Neden derleme adımı olmayan arayüz?** Toplam ~2000 satır CSS+JS. Webpack,
  npm ve `node_modules` taşımak, çözdüğü problemden büyük olurdu.

---

## Geliştirme

```bash
go test ./...                 # testler
go vet ./...                  # statik denetim
go run .                      # çalıştır
```

İki şeye dikkat:

- Arayüz dosyaları binary'nin **içine gömülüdür** (`embed.FS`). `web/` altında
  bir şey değiştirdiyseniz yeniden derlemeniz gerekir.
- Depoda tek bir ikili dosya yok. Simgeler `internal/icon` içinde **kodla
  çizilir**; PNG'ler istendiğinde üretilip bellekte tutulur, Windows tepsisi
  için gereken `.ico` ise ilk açılışta veri klasörüne yazılır (~30 ms).
- Kaynak kodundaki yorumlar İngilizce — proje açık kaynak ve katkı uluslararası.

---

## Lisans

[GPL-3.0](LICENSE) — özgür yazılım. Kullanabilir, inceleyebilir, paylaşabilir ve
değiştirebilirsiniz; değiştirdiğiniz sürümü dağıtırsanız o da aynı lisansla
kalmalıdır.
