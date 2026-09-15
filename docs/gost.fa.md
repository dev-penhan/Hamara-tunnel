# اتصال و استفاده از GOST

[GOST](https://github.com/go-gost/gost) یک ابزار Go برای proxy، port forwarding و proxy chain است. نسخه پیش‌فرض Hamara برای تونل لایه سوم و رمزنگاری‌شده WireGuard است؛ GOST برای forwarding در سطح برنامه استفاده می‌شود.

## انتخاب روش مناسب

| نیاز | ابزار پیشنهادی |
|---|---|
| اتصال کامل و رمزنگاری‌شده VPS به VPS | WireGuard |
| انتقال چند پورت TCP | GOST یا nftables |
| انتقال UDP در سطح برنامه | GOST |
| دسترسی معکوس به سرویس داخلی | GOST reverse proxy با ACL محدود |
| انتقال کل route پیش‌فرض | WireGuard |

یک TCP forward ساده، کل IP traffic، ICMP و همه برنامه‌های UDP را منتقل نمی‌کند.

## نصب امن

فایل باینری را فقط از release رسمی GOST بگیرید و checksum همان release را بررسی کنید. نسخه رسمی GOST در زمان تدوین این راهنما از مسیر زیر قابل دریافت است:

```text
https://github.com/go-gost/gost/releases
```

نسخه و معماری را از روی release انتخاب کنید و هیچ باینری تأییدنشده‌ای را روی سرور production اجرا نکنید.

## نمونه TCP forwarding

دستور زیر پورت محلی را به سرویس VPS خارج روی آدرس تونل متصل می‌کند:

```bash
gost -L tcp://127.0.0.1:18080/10.77.0.1:8080
```

اگر listener را عمومی می‌کنید، حتماً فایروال، authentication و ACL قرار دهید. GOST را به‌صورت open proxy عمومی اجرا نکنید.

## نکات مصرف منابع

- برای چند سرویس، یک process با چند service بهتر از اجرای process جدا برای هر پورت است.
- listener را روی loopback یا آدرس WireGuard قرار دهید، مگر اینکه دسترسی عمومی لازم باشد.
- bufferها را تا زمان انجام benchmark تغییر ندهید.
- timeout مناسب برای اتصال و idle تعیین کنید.
- تعداد connection و file descriptor را بررسی کنید.
- API و metrics را بدون authentication روی اینترنت باز نکنید.

## ارتباط با 3x-ui

معماری ساده‌تر معمولاً این است:

```text
3x-ui/Xray → route سیستم‌عامل → WireGuard → VPS خارج
```

GOST را زمانی اضافه کنید که به forwarding یا proxy chain مشخصی نیاز دارید. برای راهنمای کامل، فایل `docs/3x-ui.fa.md` را ببینید.

## منابع رسمی

- [سایت رسمی GOST](https://gost.run/en/)
- [مخزن رسمی GOST](https://github.com/go-gost/gost)
- [راهنمای رسمی port forwarding](https://gost.run/en/tutorials/port-forwarding/)
