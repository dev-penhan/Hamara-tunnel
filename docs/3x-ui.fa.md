# اتصال Hamara Tunnel به 3x-ui

> نسخه هدف این راهنما: **3x-ui v3.8.0** که در تاریخ 2026-09-14 منتشر شده است. قبل از ارتقای production صفحه release رسمی را بررسی کنید.
>
> پروژه رسمی: https://github.com/MHSanaei/3x-ui
صفحه release: https://github.com/MHSanaei/3x-ui/releases/tag/v3.8.0

این فایل توضیح می‌دهد چگونه 3x-ui را روی تونل فعال استفاده کنید. WireGuard داخل 3x-ui تنظیم نمی‌شود؛ WireGuard متعلق به سیستم‌عامل است و 3x-ui فقط Xray را مدیریت می‌کند.

## ترتیب پیشنهادی

1. روی هر دو VPS، Hamara Tunnel را نصب کنید.
2. با `sudo wg show` از وجود handshake مطمئن شوید.
3. ارتباط IP تونل را تست کنید، مثلاً از سمت ایران `ping 10.77.0.1`.
4. مسیر را با `ip route get <destination>` بررسی کنید.
5. سپس 3x-ui و inbound/outbound را تنظیم کنید.
6. بعد از تست موفق، قوانین فایروال را محدودتر کنید.

## نصب و اتصال 3x-ui

ساده‌ترین معماری این است که 3x-ui و Xray روی VPS خارج نصب شوند و Hamara فقط transport خصوصی بین دو VPS باشد:

```text
VPS ایران (Hamara client) === WireGuard === VPS خارج (Hamara server + 3x-ui/Xray)
```

نصب 3x-ui را از installer رسمی upstream انجام دهید:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/MHSanaei/3x-ui/master/install.sh)
sudo x-ui
```

بعد از ورود اولیه، username، password، مسیر پنل و پورت مدیریت را تغییر دهید و پورت پنل را تا حد امکان به IP مدیریت یا interface خصوصی محدود کنید. پورت پنل با پورت تونل یکی نیست.

در پنل وارد بخش `Inbounds` شوید، یک inbound بسازید، protocol و transport مناسب برنامه مجازتان را انتخاب کنید، client را بسازید و link یا QR تولیدشده را در برنامه کلاینت وارد کنید. نام گزینه‌ها ممکن است بین buildهای v3.8.x کمی متفاوت باشد.

برای اینکه inbound از مسیر Hamara قابل دسترسی باشد، روی VPS خارج بررسی کنید:

```bash
ss -lntup
ip addr show wg0
```

- برای سرویس خصوصی بین دو VPS، listener را روی `10.77.0.1` قرار دهید.
- برای inbound عمومی، آن را روی public address یا `0.0.0.0` قرار دهید و فقط پورت لازم را در فایروال باز کنید.
- خود پنل را بدون نیاز روی اینترنت عمومی bind نکنید.

در سمت ایران، کاربر نباید WireGuard را مثل subscription مربوط به Xray وارد کند. کاربر link یا QR مربوط به Xray را از 3x-ui دریافت می‌کند و route سیستم‌عامل طبق `AllowedIPs` ترافیک را از Hamara عبور می‌دهد.

اگر 3x-ui روی VPS ایران نصب شده است، جای applicationها را در معماری برعکس کنید و برای سرویس‌هایی که باید به VPS خارج برسند از route یا GOST port forwarding محدود استفاده کنید. دو default route رقیب ایجاد نکنید.

## سمت خارج به‌عنوان خروجی

روی VPS خارج این موارد باید درست باشند:

```bash
sysctl net.ipv4.ip_forward
ip route show default
iptables -t nat -S POSTROUTING
```

این سرور باید default route، forwarding فعال و در صورت نیاز masquerading برای subnet تونل داشته باشد.

## Bind و Routing

- اگر سرویس باید از سمت تونل دیده شود، آن را روی آدرس تونل مانند `10.77.0.1` یا روی آدرس مناسب bind کنید.
- برای مسیرهای مشخص از routing rules خود Xray استفاده کنید.
- برای مسیرهای ساده و کلی از route سیستم‌عامل استفاده کنید.
- endpoint وایرگارد را داخل مسیر تونل نیندازید، چون loop ایجاد می‌شود.
- برای شروع به‌جای `0.0.0.0/0` از `AllowedIPs` محدود استفاده کنید.

## دستورات بررسی

```bash
ss -lntup
sudo wg show
ip addr show wg0
ip route
sudo journalctl -u wg-quick@wg0 --no-pager
```

اگر Xray فقط روی `127.0.0.1` گوش دهد، از طریق آدرس تونل قابل دسترسی نخواهد بود.

## مشکلات رایج

### Xray محلی کار می‌کند اما از تونل نه

Bind address، route، فایروال و listening روی آدرس `10.77.0.1` یا `0.0.0.0` را بررسی کنید.

### اتصال Xray برقرار است اما اینترنت ندارد

ابتدا forwarding و NAT VPS خارج را مستقل از 3x-ui تست کنید. همچنین شمارنده‌های `wg show` را بررسی کنید.

### Full route باعث loop شده

در زمان راه‌اندازی از split route استفاده کنید و مسیر endpoint را از WAN نگه دارید.

### تنظیمات پنل از بین می‌رود

فایل‌های تولیدشده Xray را دستی ویرایش نکنید؛ تغییرات را از داخل 3x-ui انجام دهید. تنظیمات شبکه Hamara در `/etc/wireguard/` باقی می‌ماند.
