# Hamara Tunnel — راهنمای فارسی

Hamara Tunnel یک ابزار ماژولار برای اتصال امن و مجاز بین VPSهایی است که مالک یا مدیر آن‌ها هستید. بخش‌های نصب، مسیریابی، فایروال، بررسی سلامت، پشتیبان‌گیری و اتصال به 3x-ui از هم جدا شده‌اند.

## محدوده پروژه

روش پیش‌فرض پروژه WireGuard است و برای ارتباط لایه سوم رمزنگاری‌شده استفاده می‌شود. GRE و IPIP به‌تنهایی رمزنگاری ندارند و فقط باید داخل یک لایه امن یا شبکه‌ای که خودتان کنترل می‌کنید استفاده شوند. این پروژه شامل جعل پروتکل، استتار ترافیک یا قابلیت مخصوص دورزدن DPI نیست.

## Hamara Tunnel چه کاری انجام می‌دهد؟

Hamara Tunnel یک wrapper عملیاتی روی قابلیت‌های استاندارد شبکه Linux است. این پروژه interface تونل را می‌سازد، subnet بدون overlap اختصاص می‌دهد، forwarding و NAT لازم را فعال می‌کند و interface را با systemd پایدار نگه می‌دارد. تنظیم `AllowedIPs` و route تعیین می‌کنند کدام ترافیک از تونل عبور کند؛ هر برنامه‌ای به‌صورت خودکار full tunnel نمی‌شود.

هر deployment سالم چهار لایه دارد:

1. **Transport:** handshake وایرگارد یا اتصال سرویس GOST.
2. **Interface:** آدرس تونل در `ip addr` وجود داشته باشد.
3. **Routing:** دستور `ip route get DESTINATION` مسیر درست را نشان دهد.
4. **Policy:** forwarding، NAT، فایروال provider، فایروال سیستم و listener برنامه اجازه عبور دهند.

عیب‌یابی را به همین ترتیب انجام دهید و تا وقتی لایه اول و دوم سالم نشده‌اند، تنظیمات 3x-ui را تغییر ندهید.

## ساختار پروژه

```text
hamara-tunnel/
├── hamara-tunnel.sh          # نصب‌کننده تعاملی WireGuard
├── hamara-tunnel.sh          # نصب‌کننده و CLI یکپارچه مدیریت
├── scripts/
│   ├── create-ip-tunnel.sh    # ساخت GRE/IPIP مجاز
│   └── healthcheck.sh         # بررسی غیرمخرب تونل
├── docs/
│   ├── 3x-ui.en.md            # راهنمای 3x-ui v3.8.x
│   ├── 3x-ui.fa.md
│   ├── gost.en.md              # راهنمای اختیاری GOST v3
│   ├── gost.fa.md
│   └── gost-forward.example.yaml
├── README.en.md
├── README.fa.md
└── config/hamara.example.env
```

## وضعیت سازگاری نسخه‌ها

- **3x-ui:** راهنمای جداگانه با آخرین نسخه upstream موجود هنگام به‌روزرسانی این پروژه، یعنی **v3.8.0** منتشرشده در 2026-09-14، هماهنگ شده است. قبل از ارتقای production حتماً release note جدید را بررسی کنید.
- **GOST:** از پروژه رسمی `go-gost/gost` و مستندات v3 استفاده کنید. repository قدیمی `ginuerzh/gost` در حالت maintenance است.
- **WireGuard:** روش پیش‌فرض برای شبکه رمزنگاری‌شده و routed باقی می‌ماند.

## روش‌های موجود

### WireGuard Standard
روش پیشنهادی برای بیشتر سناریوها؛ سریع، ساده و رمزنگاری‌شده.

### WireGuard Full Tunnel
ترافیک شبکه‌های مشخص یا کل مسیر پیش‌فرض از VPS خارج عبور می‌کند. ابتدا Split Tunnel را تست کنید و بعد سراغ default route بروید.

### Hamara Multi-Tunnel
دو اینترفیس مستقل WireGuard با پورت و subnet جدا. می‌توانید یکی را برای مسیر اصلی و دیگری را برای failover کنترل‌شده استفاده کنید؛ مراقب loop مسیریابی باشید.

### ARA IP-forwarding
پروفایل اختصاصی با اینترفیس `ara0`، forwarding صریح، NAT، MTU، keepalive و health check. ARA یک پروفایل مسیریابی است و پروتکل ضد DPI نیست.

### GRE و IPIP
اسکریپت `scripts/create-ip-tunnel.sh` برای شبکه‌هایی است که خودتان کنترل می‌کنید. این دو روش به‌تنهایی رمزنگاری ندارند؛ در صورت نیاز آن‌ها را داخل WireGuard قرار دهید.

## نصب

repository رسمی پروژه: `https://github.com/dev-penhan/Hamara-tunnel`

### نصب با Git

```bash
git clone https://github.com/dev-penhan/Hamara-tunnel hamara-tunnel
cd hamara-tunnel
chmod +x hamara-tunnel.sh
sudo ./hamara-tunnel.sh install
```

بعد از نصب، دستور سراسری زیر ساخته می‌شود و کاربر فقط آن را اجرا می‌کند:

```bash
hamara
hamara status
hamara doctor
```

کاربر لازم نیست `sudo hamara` بنویسد؛ launcher خودش در زمان نیاز دسترسی root را درخواست می‌کند.

### نصب مستقیم با لینک raw

```bash
curl -fsSL https://raw.githubusercontent.com/dev-penhan/Hamara-tunnel/main/hamara-tunnel.sh -o /tmp/hamara
chmod +x /tmp/hamara
sudo /tmp/hamara install
```

در VPS خارج نقش `Foreign-side / exit VPS` و در VPS ایران نقش `Iran-side / client-side VPS` را انتخاب کنید. پروفایل و subnet باید دو طرف یکسان باشد.

پورت‌های پیش‌فرض:

- `wg0`: UDP 51820
- `wg1`: UDP 51821
- `ara0`: UDP 51830

پورت انتخاب‌شده باید هم در فایروال Cloud Provider و هم در فایروال سیستم باز باشد.

## چک‌لیست عملیاتی

1. سیستم‌عامل و بسته‌ها را به‌روز نگه دارید.
2. ورود SSH با رمز عبور را غیرفعال و از کلید استفاده کنید.
3. در صورت امکان پورت WireGuard را به IPهای مشخص محدود کنید.
4. ابتدا split route را تست کنید.
5. subnetهای تکراری یا overlap نداشته باشید.
6. در صورت مشکل packet loss مقدار MTU را بررسی کنید.
7. از فایل‌های تنظیمات به‌صورت امن backup بگیرید.
8. handshake و شمارنده ترافیک را مانیتور کنید.
9. endpoint خود WireGuard را وارد مسیر تونل نکنید.

## دستورات مدیریت

برای ساخت یک TCP forward محلی و محدود به loopback با GOST:

```bash
hamara gost
```

این دستور وجود GOST را بررسی می‌کند، یک سرویس systemd می‌سازد و به‌صورت پیش‌فرض listener را روی loopback قرار می‌دهد؛ open proxy عمومی ایجاد نمی‌کند. راهنمای کامل در `docs/gost.fa.md` است.

```bash
sudo ./hamara-tunnel.sh status
sudo ./hamara-tunnel.sh doctor
hamara health
hamara audit
hamara backup /root/hamara-backups
```

فایل backup شامل کلید خصوصی است و باید مانند رمز عبور نگهداری شود.

## درخت عیب‌یابی

این دستورات را به‌ترتیب اجرا کنید و خروجی را قبل از تغییرات ذخیره کنید:

```bash
sudo hamara doctor
sudo hamara status
ip -br addr
ip route
sudo wg show
ss -lunp
```

- **Interface ساخته نشده:** سرویس یا config بالا نیامده؛ `systemctl status wg-quick@wg0` و journal را ببینید.
- **Interface وجود دارد اما handshake نیست:** endpoint، کلیدهای عمومی، پورت UDP در Cloud، فایروال، ساعت سیستم و keepalive را بررسی کنید.
- **Handshake هست اما ping تونل نیست:** آدرس تونل، `AllowedIPs`، overlap route و policy ورودی/forwarding را بررسی کنید.
- **Ping تونل هست اما اینترنت نیست:** default route و forwarding و NAT و مسیر برگشت VPS خارج را جداگانه بررسی کنید؛ DNS را هم مستقل تست کنید.
- **فقط 3x-ui مشکل دارد:** تنظیمات Hamara را تغییر ندهید؛ listener و bind address و لاگ Xray و outbound را بررسی کنید.
- **ترافیک ناپایدار است:** MTU، packet loss، مصرف CPU و شمارنده‌های `wg show` را بررسی کنید.

## عیب‌یابی

- نبودن handshake: پورت UDP، فایروال Cloud، endpoint، کلیدها، ساعت سیستم و keepalive را بررسی کنید.
- handshake بدون ترافیک: `AllowedIPs`، routeها، forwarding و فایروال را بررسی کنید.
- قطع شدن SSH در full tunnel: ابتدا split route را فعال کنید و مسیر مدیریت را از WAN نگه دارید.
- مشکل DNS: تونل خودش DNS server نیست؛ resolver مناسب تنظیم کنید.
- مشکل 3x-ui: ابتدا خود WireGuard را تست کنید، سپس لاگ Xray و listener را بررسی کنید.

آموزش کامل 3x-ui در فایل جداگانه `docs/3x-ui.fa.md` قرار دارد.

## بررسی پروژه‌های خارجی

دو پروژه پیشنهادی بررسی شدند:

- [paqet-tunnel](https://github.com/g3ntrix/paqet-tunnel) از raw-packet tunneling استفاده می‌کند و صراحتاً برای bypass محدودیت‌های شبکه طراحی شده است. کد آن را وارد Hamara نکردم، چون raw packet و رفتار ضد DPI نیاز به threat model، تست kernel/provider و بررسی ریسک جداگانه دارد.
- [Pahlavi-tunnel](https://github.com/Zehnovik/Pahlavi-tunnel) یک reverse TCP tunnel manager با multi-slot، همگام‌سازی، health check، systemd و forwarding است. ایده‌های عملی آن مثل health check، restart policy، slot isolation و کنترل منابع در طراحی Hamara لحاظ شده، اما کدش vendored نشده است.

Hamara ادعا نمی‌کند هیچ تونلی از inspection نامرئی است. هر transport را فقط روی زیرساختی که مدیریت می‌کنید تست و نتیجه را مستند کنید.
