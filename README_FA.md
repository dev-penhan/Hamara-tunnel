# راهنمای فارسی Hamara Tunnel

[English](README.md) · [مخزن رسمی پروژه](https://github.com/dev-penhan/Hamara-tunnel)

**Hamara Tunnel** یک ابزار متن‌باز برای ساخت مسیر واقعی بین دو VPS اوبونتو است:

- **سرور ایران (Iran edge):** سروری که 3x-ui/Xray و ورودی کاربران روی آن قرار دارد.
- **سرور خارج (Outside gateway):** سروری که خروجی اینترنت از آن انجام می‌شود.

بعد از نصب کافی است بنویسید:

```bash
hamara
```

برنامه در صورت نیاز خودش `sudo` را اجرا می‌کند و منوی نصب یا مدیریت را نمایش می‌دهد. برای دیدن Help یا Version نیازی به دسترسی root نیست.

> **هشدار صادقانه:** هیچ تانلی را نمی‌توان در همه شبکه‌ها «کاملاً نامرئی»، «صددرصد ضد DPI» یا «بدون هیچ احتمال نشت» تضمین کرد. ARA تلاش می‌کند WireGuard را مستقیماً در اینترنت نمایش ندهد و آن را داخل TLS/WebSocket قرار دهد، اما مسدودسازی IP، تحلیل رفتار، Active Probing، تحلیل زمان/حجم ترافیک و اختلال عمدی همچنان ممکن است. قابلیت‌های امنیتی پروژه احتمال fallback و نشت مسیر علامت‌گذاری‌شده را بسیار کمتر می‌کنند، ولی تنظیم اشتباه 3x-ui، DNS سیستم، کانتینرها یا یک outbound مستقیم جداگانه خارج از کنترل Hamara است.

## مشخصات نسخه

- نسخه: `0.2.0`
- سیستم‌عامل: Ubuntu 22.04 LTS و Ubuntu 24.04 LTS
- معماری: `amd64` و برای ARA همچنین `arm64`
- شبکه نسخه فعلی: مسیر دیتای IPv4
- سرویس‌ها: systemd
- فایروال: iptables با backend سازگار nftables اوبونتو
- هر نصب: یک سرور ایران و یک سرور خارج
- مخزن رسمی: <https://github.com/dev-penhan/Hamara-tunnel>

## روش کار کلی

```text
کاربر
  ↓
Inbound در 3x-ui/Xray روی سرور ایران
  ↓
Outbound با نام hamara-egress و mark=72
  ↓
hamara-guard + جدول Policy Routing اختصاصی
  ↓
رابط hamara0
  ↓
ARA / WireGuard / IPIP
  ↓
سرور خارج
  ↓
IP Forwarding + NAT
  ↓
اینترنت
```

Hamara جایگزین 3x-ui نیست. پروتکل کاربران مانند VLESS، VMess، Trojan یا Shadowsocks همچنان توسط پنل مدیریت می‌شود. Hamara مسیر خروجی بین دو سرور را فراهم می‌کند.

## روش‌های تانل موجود

| روش | رمزنگاری | TCP | UDP | کاربرد | محدودیت اصلی |
|---|---:|---:|---:|---|---|
| **ARA Routed TLS** | WireGuard + TLS + mTLS | بله | بله | روش پیشنهادی وقتی WireGuard مستقیم فیلتر می‌شود | سربار بیشتر و افت کارایی احتمالی روی شبکه دارای Packet Loss |
| **WireGuard Direct** | WireGuard + PSK | بله | بله | سریع‌ترین روش عمومی | UDP/WireGuard مستقیماً قابل مشاهده است |
| **IPIP Routed** | ندارد | بله | بله | مسیر سبک یا تست شبکه مورد اعتماد | بدون محرمانگی، IPv4، نیازمند IP مستقیم و Protocol 4 |
| **SSH SOCKS** | SSH | بله | خیر | مسیر جایگزین TCP | برای UDP، بازی و QUIC مناسب نیست |

## ARA Tunnel دقیقاً چه می‌کند؟

ARA از رمزنگاری اختراعی استفاده نمی‌کند و چند لایه استاندارد را ترکیب می‌کند:

1. Xray خروجی انتخاب‌شده را به IP سمت ایران روی `hamara0` متصل می‌کند.
2. Xray روی سوکت خروجی `mark=72` قرار می‌دهد.
3. سرویس `hamara-guard` ترافیک دارای این mark یا Source IP تانل را فقط وارد جدول Route اختصاصی می‌کند.
4. داخل آن جدول همیشه یک `unreachable default` وجود دارد.
5. وقتی تانل سالم است، مسیر `hamara0` با Metric کمتر فعال می‌شود.
6. اگر تانل قطع شود، Route به جدول اصلی اینترنت ایران fallback نمی‌کند و اتصال Fail Closed می‌شود.
7. WireGuard دیتای Layer 3 را با کلیدهای طرفین و PSK رمز می‌کند.
8. در ARA، UDP داخلی WireGuard به listener لوکال فرستاده می‌شود.
9. `wstunnel` آن را داخل WSS/TLS به سرور خارج می‌برد.
10. سرور خارج علاوه بر TLS، گواهی اختصاصی mTLS کلاینت را نیز بررسی می‌کند.
11. CN گواهی mTLS با مسیر تصادفی WebSocket یکسان است.
12. مقصد قابل Forward توسط wstunnel فقط `127.0.0.1:51820` است.
13. در پایان، Kernel سرور خارج IP Forwarding و NAT را انجام می‌دهد.

### امنیت mTLS در ARA

برای هر نصب ARA یک CA و Client Certificate از نوع P-256 ساخته می‌شود:

- سرور خارج فقط کلاینتی را قبول می‌کند که گواهی امضاشده توسط همان CA را داشته باشد.
- Common Name گواهی با ARA Path تصادفی برابر است.
- کلید CA و نسخه کلید خصوصی کلاینت بعد از تولید OFFER از فایل‌های عادی سرور خارج حذف می‌شوند.
- کلید انتقالی فقط داخل فایل root-only مربوط به OFFER باقی می‌ماند.
- روی سرور ایران، Certificate و Key در `/etc/hamara/mtls/` ذخیره می‌شوند.

### حالت بدون دامنه

اگر فقط IP دارید می‌توانید Self-signed را انتخاب کنید. در این حالت:

- mTLS برای احراز هویت سرور ایران همچنان فعال است.
- WireGuard همچنان دیتای داخلی را احراز هویت و رمز می‌کند.
- ولی گواهی بیرونی سرور خارج توسط کلاینت Verify نمی‌شود.

بنابراین دامنه واقعی و گواهی معتبر Let's Encrypt یا گواهی موجود، انتخاب امن‌تر است.

## جلوگیری از نشت IP و fallback

در روش‌های Routed روی سرور ایران، سه کنترل هم‌زمان استفاده می‌شود:

```text
sendThrough       = IP داخلی Hamara
sockopt.interface = hamara0
sockopt.mark      = 72
```

سرویس `hamara-guard` نیز موارد زیر را فعال می‌کند:

- Rule برای `fwmark 72`
- Rule برای Source IP داخلی تانل
- جدول Routing اختصاصی
- مسیر `unreachable default` که حتی هنگام قطع تانل باقی می‌ماند
- REJECT کردن IPv4 علامت‌گذاری‌شده در صورت تلاش برای خروج از WAN ایران
- REJECT کردن Source IP تانل در صورت تلاش برای خروج از رابطی غیر از `hamara0`
- مسدودکردن IPv6 دارای mark، چون نسخه فعلی مسیر IPv6 ندارد
- وابسته‌کردن سرویس WireGuard/IPIP به `hamara-guard`

وقتی روی سرور ایران دستور زیر اجرا شود:

```bash
hamara stop
```

Transport متوقف می‌شود ولی `hamara-guard` روشن باقی می‌ماند تا ترافیک Xray علامت‌گذاری‌شده از اینترنت عادی ایران خارج نشود.

### تست خودکار نشت

روی سرور ایران و در Maintenance Window اجرا کنید:

```bash
hamara leak-test
```

این تست:

1. خروجی سالم تانل را بررسی می‌کند.
2. رابط تانل را موقتاً متوقف می‌کند.
3. `hamara-guard` را روشن نگه می‌دارد.
4. Route مربوط به mark 72 را بررسی می‌کند.
5. تلاش می‌کند با Source IP تانل درخواست HTTP بفرستد؛ این درخواست باید شکست بخورد.
6. سرویس تانل را دوباره روشن می‌کند.

بعد از آن باید یک کاربر واقعی 3x-ui را هم هنگام قطع تانل تست کنید. اگر Rule اشتباه در 3x-ui قبل از Rule Hamara قرار گرفته باشد، Hamara نمی‌تواند ترتیب Rule پنل را از بیرون اصلاح کند.

## پیش‌نیازها

روی هر دو VPS:

- Ubuntu 22.04 یا 24.04
- دسترسی root یا کاربر دارای sudo
- IPv4 عمومی و Route سالم
- systemd
- ساعت صحیح سیستم
- امکان تغییر فایروال داخل VPS
- بازبودن پورت/پروتکل لازم در Firewall شرکت ارائه‌دهنده VPS

برای ARA پیشنهاد می‌شود:

- یک رکورد A مانند `tunnel.example.com` روی IP سرور خارج
- بازبودن TCP 443
- بازبودن موقت TCP 80 برای دریافت Let's Encrypt
- آزادبودن پورت انتخاب‌شده روی سرور خارج

استفاده 3x-ui از پورت 443 روی **سرور ایران** مشکلی ایجاد نمی‌کند، چون Listener عمومی ARA روی **سرور خارج** است.

## نصب از GitHub

روی هر دو سرور:

```bash
git clone https://github.com/dev-penhan/Hamara-tunnel.git
cd Hamara-tunnel
sudo ./install.sh
hamara
```

نصب یک‌خطی:

```bash
curl -fsSL https://raw.githubusercontent.com/dev-penhan/Hamara-tunnel/main/scripts/quick-install.sh | sudo bash
hamara
```

فایل اجرایی در مسیر زیر نصب می‌شود:

```text
/usr/local/bin/hamara
```

از این مرحله به بعد نیازی نیست قبل از دستور Hamara کلمه `sudo` بنویسید. خود برنامه در زمان لازم پنجره درخواست sudo را باز می‌کند. کاربر Linux باید اجازه sudo داشته باشد.

## راه‌اندازی ساده ARA

### مرحله ۱: DNS

یک رکورد A بسازید:

```text
tunnel.example.com -> OUTSIDE_VPS_IPV4
```

بررسی:

```bash
dig +short tunnel.example.com A
```

### مرحله ۲: سرور خارج

```bash
hamara
```

انتخاب‌ها:

```text
Which server is this?       Outside gateway server
Choose a tunnel mode        ARA Routed TLS
Public endpoint             tunnel.example.com
Public ARA TLS port         443
TLS certificate mode        Let's Encrypt
```

در پایان توکن زیر ساخته می‌شود:

```text
HAMARA1.OFFER....
```

**OFFER محرمانه است** و شامل WireGuard PSK و کلید خصوصی یک‌بار صادرشده mTLS است. آن را در کانال امن به سرور ایران منتقل کنید.

### مرحله ۳: سرور ایران

```bash
hamara
```

گزینه `Iran edge server` را بزنید و OFFER را Paste کنید. خروجی:

```text
HAMARA1.RESPONSE....
```

### مرحله ۴: Accept روی سرور خارج

برای جلوگیری از ثبت Token در Shell History، دستور `hamara` را اجرا کنید، گزینه **Accept a pairing response** را انتخاب کنید و RESPONSE را Paste کنید.

برای Automation، فایل root-only استفاده کنید:

```bash
chmod 600 /root/hamara-response.txt
hamara accept --file /root/hamara-response.txt
```

### مرحله ۵: بررسی

روی هر دو سرور:

```bash
hamara status
hamara doctor
```

روی سرور ایران:

```bash
hamara leak-test
```

## نصب Non-interactive

برای Automation بهتر است توکن‌ها را داخل فایل با Permission برابر `0600` قرار دهید. استفاده مستقیم از `--offer` ممکن است Secret را در Shell History یا Process List نمایش دهد.

### ARA با Let's Encrypt

سرور خارج:

```bash
hamara init \
  --role outside \
  --mode ara \
  --endpoint tunnel.example.com \
  --port 443 \
  --tls acme \
  --email admin@example.com
```

سرور ایران پس از انتقال OFFER:

```bash
chmod 600 /root/hamara-offer.txt
hamara join --file /root/hamara-offer.txt
```

سرور خارج پس از انتقال RESPONSE:

```bash
chmod 600 /root/hamara-response.txt
hamara accept --file /root/hamara-response.txt
```

### ARA با Certificate موجود

```bash
hamara init \
  --role outside \
  --mode ara \
  --endpoint tunnel.example.com \
  --port 443 \
  --tls existing \
  --cert /etc/letsencrypt/live/tunnel.example.com/fullchain.pem \
  --key /etc/letsencrypt/live/tunnel.example.com/privkey.pem
```

### ARA فقط با IP

```bash
hamara init \
  --role outside \
  --mode ara \
  --endpoint 203.0.113.10 \
  --port 443 \
  --tls self-signed
```

### WireGuard مستقیم

```bash
hamara init \
  --role outside \
  --mode wireguard \
  --endpoint 203.0.113.10 \
  --port 51820
```

در فایروال Provider باید UDP 51820 باز باشد.

### IPIP

```bash
hamara init \
  --role outside \
  --mode ipip \
  --endpoint 203.0.113.10
```

IPIP به IP مستقیم روی هر دو VPS و بازبودن IP Protocol 4 نیاز دارد. از NAT معمولی عبور نمی‌کند و رمزنگاری ندارد.

### SSH SOCKS

```bash
hamara init \
  --role outside \
  --mode ssh-socks \
  --endpoint 203.0.113.10 \
  --port 22
```

این روش فقط TCP است. سرویس SOCKS روی سرور ایران فقط به آدرس زیر Bind می‌شود:

```text
127.0.0.1:10808
```

## منوی مدیریت

با اجرای ساده:

```bash
hamara
```

منوی زیر در دسترس است:

- نمایش وضعیت
- Start سرویس‌ها
- Stop سرویس‌ها بدون خاموش‌کردن Guard
- Restart
- اجرای Doctor
- اجرای Leak Test
- تولید تنظیمات 3x-ui/Xray
- نمایش Logها
- Repair فایروال و Guard
- Accept کردن RESPONSE
- نمایش دوباره Pairing Token
- نمایش مسیر تمام فایل‌های ذخیره‌شده
- Uninstall

## دستورهای مدیریت

```bash
hamara status
hamara start
hamara stop
hamara restart
hamara doctor
hamara leak-test
hamara logs
hamara repair
hamara paths
hamara xray
hamara pairing-token
hamara uninstall
```

توضیح:

| دستور | کاربرد |
|---|---|
| `hamara status` | وضعیت Role، Mode، Serviceها و Handshake |
| `hamara start` | روشن‌کردن سرویس‌های تانل با رعایت ترتیب امنیتی |
| `hamara stop` | خاموش‌کردن Transport؛ Guard سمت ایران روشن می‌ماند |
| `hamara restart` | Restart کنترل‌شده سرویس‌ها |
| `hamara doctor` | بررسی IP forwarding، Guard، Interface و Egress |
| `hamara leak-test` | تست Fail Closed هنگام قطع رابط تانل |
| `hamara logs` | نمایش ۲۰۰ خط آخر Log سرویس‌های مربوط |
| `hamara repair` | اعمال دوباره NAT/Firewall یا Strict Guard |
| `hamara paths` | نمایش مسیر و وضعیت فایل‌های ذخیره‌شده |
| `hamara xray` | ساخت JSON مناسب 3x-ui |
| `hamara pairing-token` | نمایش دوباره OFFER یا RESPONSE |
| `hamara uninstall` | حذف تنظیمات و سرویس‌ها |

## تنظیم 3x-ui/Xray برای کاربر مبتدی

این تنظیمات باید روی **سرور ایران** انجام شود و ابتدا باید دستور زیر موفق باشد:

```bash
hamara doctor
```

### تولید JSON دقیق

```bash
hamara xray
```

برای ARA/WireGuard/IPIP خروجی شبیه این است:

```json
{
  "tag": "hamara-egress",
  "protocol": "freedom",
  "sendThrough": "10.73.0.2",
  "settings": {},
  "streamSettings": {
    "sockopt": {
      "interface": "hamara0",
      "mark": 72,
      "domainStrategy": "UseIPv4",
      "tcpFastOpen": true
    }
  }
}
```

عدد IP را از این README کپی نکنید؛ خروجی واقعی `hamara xray` را استفاده کنید.

### افزودن Outbound

1. وارد 3x-ui روی سرور ایران شوید.
2. وارد **Xray Settings → Outbounds** شوید.
3. یک Outbound جدید بسازید یا بخش JSON را باز کنید.
4. Object تولیدشده توسط `hamara xray` را Paste کنید.
5. Tag باید دقیقاً `hamara-egress` باشد.
6. Outboundهای `direct`، `block` و `api` را حذف نکنید.

### Route تمام کاربران از Hamara

Rule زیر را اضافه کنید:

```json
{
  "type": "field",
  "network": "tcp,udp",
  "outboundTag": "hamara-egress"
}
```

ترتیب Ruleها مهم است:

- Rule مربوط به API پنل باید بالاتر باشد.
- Ruleهای Block موردنظر باید بالاتر باشند.
- Rule Hamara باید قبل از هر Rule عمومی Direct قرار بگیرد.
- برای SSH SOCKS مقدار Network باید فقط `tcp` باشد.

### Route فقط چند Inbound

```bash
hamara xray --inbound-tags inbound-443,inbound-test
```

از Tag واقعی Inbound استفاده کنید، نه لزوماً Remark نمایشی پنل.

### تست نهایی پنل

1. Xray را از پنل Restart کنید.
2. Outbound Test و Route Test پنل را اجرا کنید.
3. یک کاربر آزمایشی وصل کنید.
4. IP خروجی را بررسی کنید.
5. در Maintenance Window دستور `hamara leak-test` را اجرا کنید.
6. هنگام قطع تانل، اتصال کاربر باید Fail شود و نباید IP سرور ایران را نشان دهد.

### نکته DNS

Hamara مسیر Outbound علامت‌گذاری‌شده Xray را محافظت می‌کند، اما همه DNSهای سیستم را خودکار Capture نمی‌کند. اگر DNS Privacy مهم است:

- DNS/DoH معتبر را در Xray تنظیم کنید.
- Ruleهای DNS را بررسی کنید.
- با Packet Capture و Leak Test واقعی کنترل کنید.
- فرض نکنید Freedom Outbound به تنهایی تمام DNSهای سیستم را تغییر می‌دهد.

## پورت‌ها و پروتکل‌ها

| Mode | ورودی سرور خارج | Listener داخلی | توضیح |
|---|---|---|---|
| ARA | TCP 443 پیش‌فرض | UDP 51820 خارج و UDP 51821 ایران روی Loopback | دسترسی WAN مستقیم به UDP داخلی خارج مسدود می‌شود |
| WireGuard | UDP 51820 | ندارد | قابل تغییر است |
| IPIP | IP Protocol 4 | ندارد | منظور TCP/UDP Port 4 نیست |
| SSH SOCKS | پورت فعلی SSH | `127.0.0.1:10808` روی ایران | فقط TCP |

فایروال Provider خارج از کنترل Hamara است و باید جداگانه تنظیم شود.

## تمام فایل‌ها و محل ذخیره

در هر زمان می‌توانید اجرا کنید:

```bash
hamara paths
```

همه Modeها همه فایل‌ها را ایجاد نمی‌کنند.

| مسیر | محل/Mode | محتوای ذخیره‌شده | محرمانه؟ |
|---|---|---|---:|
| `/usr/local/bin/hamara` | هر دو سرور | فایل اجرایی CLI | خیر |
| `/usr/local/bin/wstunnel` | ARA هر دو سرور | باینری Transport با Hash بررسی‌شده | خیر |
| `/opt/hamara-tunnel/` | هر دو | کپی README، License و Docs | خیر |
| `/etc/hamara/config.json` | هر دو | Role، Mode، IP، Port، Path و State با Mode 0600 | عملیاتی |
| `/etc/hamara/offer.token` | سرور خارج | OFFER شامل WG PSK و ARA mTLS Key | **بله** |
| `/root/hamara-offer.txt` | سرور خارج | کپی راحت OFFER | **بله** |
| `/etc/hamara/response.token` | سرور ایران | RESPONSE حاوی Public Identity سمت ایران | خصوصی |
| `/root/hamara-response.txt` | سرور ایران | کپی راحت RESPONSE | خصوصی |
| `/etc/hamara/secrets/wg_private.key` | ARA/WG هر دو | کلید خصوصی WireGuard | **بله** |
| `/etc/hamara/secrets/wg_public.key` | ARA/WG هر دو | کلید عمومی WireGuard | خیر |
| `/etc/hamara/secrets/wg_preshared.key` | ARA/WG هر دو | PSK | **بله** |
| `/etc/hamara/secrets/ssh_ed25519` | SSH ایران | کلید خصوصی SSH Forwarding | **بله** |
| `/etc/hamara/mtls/client-ca.crt` | ARA خارج | CA مجاز برای کلاینت ایران | خیر |
| `/etc/hamara/mtls/client.crt` | ARA ایران | Certificate کلاینت mTLS | خصوصی |
| `/etc/hamara/mtls/client.key` | ARA ایران | کلید خصوصی P-256 برای mTLS | **بله** |
| `/etc/hamara/tls/fullchain.pem` | ARA Self-signed خارج | Certificate بیرونی | خیر |
| `/etc/hamara/tls/privkey.pem` | ARA Self-signed خارج | کلید خصوصی TLS بیرونی | **بله** |
| `/etc/hamara/xray-outbound.json` | ایران | JSON تولیدشده برای 3x-ui | بدون Secret |
| `/etc/hamara/ssh_known_hosts` | SSH ایران | Host Key پین‌شده سرور خارج | خیر |
| `/etc/wireguard/hamara0.conf` | ARA/WG هر دو | تنظیم Interface و Private/Peer Key | **بله** |
| `/etc/sysctl.d/90-hamara-tunnel.conf` | Routed | IPv4 Forwarding و rp_filter | خیر |
| `/usr/local/lib/hamara/guard` | Routed ایران | اسکریپت Fail-closed و Kill Switch | خیر |
| `/usr/local/lib/hamara/firewall` | Routed خارج | اسکریپت NAT و Forwarding | خیر |
| `/usr/local/lib/hamara/ipip` | IPIP | ساخت/حذف Interface | خیر |
| `/etc/systemd/system/hamara-guard.service` | Routed ایران | سرویس Guard | خیر |
| `/etc/systemd/system/hamara-ara-client.service` | ARA ایران | سرویس WSS/mTLS Client | فقط Pathها |
| `/etc/systemd/system/hamara-ara-server.service` | ARA خارج | سرویس WSS/mTLS Server | فقط Pathها |
| `/etc/systemd/system/hamara-firewall.service` | Routed خارج | سرویس NAT/Firewall | خیر |
| `/etc/systemd/system/hamara-ipip.service` | IPIP | سرویس IPIP | خیر |
| `/etc/systemd/system/hamara-ssh-socks.service` | SSH ایران | سرویس SOCKS | فقط Pathها |
| `/etc/systemd/system/wg-quick@hamara0.service.d/hamara-guard.conf` | ARA/WG ایران | اجبار Start Guard قبل از WG | خیر |
| `/etc/ssh/sshd_config.d/90-hamara-tunnel.conf` | SSH خارج | محدودیت User مخصوص Hamara | خیر |
| `/var/lib/hamara-ssh/.ssh/authorized_keys` | SSH خارج | Public Key کلاینت Forwarding | عمومی |

فایل‌های زیر را هیچ‌وقت منتشر نکنید:

```text
/etc/hamara/offer.token
/root/hamara-offer.txt
/etc/hamara/secrets/*
/etc/hamara/mtls/client.key
/etc/hamara/tls/privkey.pem
/etc/wireguard/hamara0.conf
```

## نام سرویس‌ها

### ARA سرور خارج

```text
wg-quick@hamara0.service
hamara-ara-server.service
hamara-firewall.service
```

### ARA سرور ایران

```text
hamara-guard.service
hamara-ara-client.service
wg-quick@hamara0.service
```

### Modeهای دیگر

```text
hamara-ipip.service
hamara-ssh-socks.service
```

## رفع اشکال سریع

ابتدا:

```bash
hamara status
hamara doctor
hamara logs
sudo systemctl --failed
```

### ARA روی Port 443 بالا نمی‌آید

سرور خارج:

```bash
sudo ss -ltnp | grep ':443 '
sudo systemctl status hamara-ara-server --no-pager
```

بررسی کنید:

- DNS روی IP سرور خارج باشد.
- TCP 443 در Provider Firewall باز باشد.
- Nginx/Caddy/HAProxy یا سرویس دیگری Port را نگرفته باشد.
- Certificate معتبر و ساعت سیستم صحیح باشد.
- اگر Port اشغال است از 8443 استفاده کنید.

### TLS وصل می‌شود ولی Handshake نداریم

سرور ایران:

```bash
sudo systemctl status hamara-guard hamara-ara-client wg-quick@hamara0 --no-pager
sudo wg show hamara0
sudo ls -l /etc/hamara/mtls/
```

سرور خارج:

```bash
sudo systemctl status hamara-ara-server wg-quick@hamara0 --no-pager
sudo wg show hamara0
```

دلایل رایج:

- RESPONSE روی سرور خارج Accept نشده است.
- OFFER و RESPONSE مربوط به دو Instance متفاوت هستند.
- OFFER ناقص Paste شده است.
- فایل mTLS Certificate یا Key سمت ایران حذف شده است.
- ساعت یکی از سرورها اشتباه است.
- شبکه اتصال طولانی WebSocket را Reset می‌کند.

### Handshake داریم ولی اینترنت نداریم

روی خارج:

```bash
sysctl net.ipv4.ip_forward
sudo iptables -S FORWARD
sudo iptables -t nat -S POSTROUTING
hamara repair
```

### Doctor موفق است ولی کاربر IP ایران می‌گیرد

احتمالاً Ruleهای Xray اشتباه هستند:

1. Tag باید `hamara-egress` باشد.
2. Routing Rule باید همین Tag را صدا بزند.
3. Rule عمومی Direct نباید قبل از Rule Hamara باشد.
4. API Rule پنل باید بالاتر بماند.
5. Xray را Restart کنید.
6. Route Test پنل را اجرا کنید.
7. `hamara leak-test` را اجرا کنید.

### Guard مشکل دارد

```bash
hamara repair
hamara doctor
sudo ip rule show
sudo ip route show table 52730
sudo iptables -S OUTPUT
```

جدول باید `unreachable default` داشته باشد. اگر Leak Test عبارت `CRITICAL` نمایش داد، ترافیک کاربران را متوقف کنید و Guard را برای وصل‌شدن ظاهری خاموش نکنید.

### دانلودها متوقف می‌شوند

ممکن است MTU مشکل داشته باشد:

```bash
ping -4 -M do -s 1200 1.1.1.1
tracepath -4 1.1.1.1
```

MTU پیش‌فرض:

- ARA: `1280`
- WireGuard: `1380`
- IPIP: `1400`

برای تست موقت:

```bash
sudo ip link set dev hamara0 mtu 1200
```

### SSH SOCKS و UDP

SSH Dynamic Forwarding فقط TCP است. برای UDP از ARA یا WireGuard استفاده کنید و در Mode SSH Rule پنل را روی `tcp` قرار دهید.

## آپدیت

```bash
cd Hamara-tunnel
git pull --ff-only
sudo ./install.sh
hamara status
```

اگر ARA با نسخه 0.1 ساخته شده است، روی هر دو سرور Uninstall و Pair مجدد انجام دهید تا mTLS و Guard نسخه 0.2 نصب شوند.

## حذف پروژه

```bash
hamara uninstall
```

برای حذف باینری wstunnel نیز:

```bash
hamara uninstall --purge-wstunnel
```

پکیج‌های Ubuntu و Certificateهای مشترک `/etc/letsencrypt` حذف نمی‌شوند. Hamara مقدارهای قبلی sysctl را در صورت امکان برمی‌گرداند.

## امنیت و نگهداری

- 3x-ui را از منبع رسمی و به‌روز نصب کنید.
- پنل را مستقیماً برای همه اینترنت باز نگذارید.
- OFFER را مثل Password نگه دارید.
- از Private Keyها Backup طولانی‌مدت نگیرید؛ Pair مجدد امن‌تر است.
- قبل از Production از `hamara doctor` و `hamara leak-test` استفاده کنید.
- یک اتصال SSH باز نگه دارید و بعد تغییرات شبکه را تست کنید.
- Logها را بدون حذف Secretها منتشر نکنید.
- IP، ASN، رفتار طولانی TLS و حجم ترافیک می‌تواند همچنان توسط شبکه شناسایی شود.

مستندات بیشتر:

- [Threat Model](docs/THREAT_MODEL.md)
- [Operations Runbook](docs/OPERATIONS.md)
- [Security Policy](SECURITY.md)

## سوالات متداول

### آیا ARA صددرصد توسط DPI شناسایی نمی‌شود؟

خیر. WireGuard مستقیم را داخل WSS/TLS قرار می‌دهد و احراز هویت mTLS و Path تصادفی دارد، اما هیچ تضمین مطلقی مقابل DPI پیشرفته، مسدودسازی Endpoint یا تحلیل رفتار وجود ندارد.

### آیا نشت IP صددرصد غیرممکن است؟

برای Outbound تولیدشده Hamara چند لایه Fail-closed فعال است، اما تنظیم اشتباه Xray، DNS جداگانه، Container، outbound بدون mark یا تغییرات root می‌تواند خارج از این کنترل باشد. تست واقعی ضروری است.

### برای شروع کدام Mode بهتر است؟

- اگر UDP آزاد است: WireGuard Direct برای سرعت
- اگر WireGuard فیلتر می‌شود: ARA
- اگر فقط TCP دارید: SSH SOCKS
- IPIP فقط وقتی نبود رمزنگاری را قبول دارید

### آیا کل VPS ایران از تانل عبور می‌کند؟

خیر. فقط برنامه‌ای که از Source/Interface/Mark تولیدشده Hamara استفاده کند وارد جدول اختصاصی می‌شود. Route اصلی SSH مدیریت تغییر نمی‌کند.

### آیا چند Inbound پنل می‌توانند استفاده کنند؟

بله. Rule عمومی یا فهرست `inboundTag`های انتخابی را استفاده کنید.

### آیا IPv6 پشتیبانی می‌شود؟

مسیر Payload نسخه 0.2 فقط IPv4 است. ترافیک IPv6 دارای mark عمداً Reject می‌شود تا fallback ایجاد نشود.

### آیا 3x-ui اجباری است؟

خیر. هر برنامه‌ای که بتواند Source Address و Interface را Bind کند می‌تواند از Routed Mode استفاده کند، ولی راهنمای آماده برای 3x-ui/Xray نوشته شده است.

## مجوز

Hamara Tunnel تحت مجوز MIT منتشر می‌شود.
