# 🎓 AbitAssistant

[![License: GPLv3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0.html)
[![Go 1.26+](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](https://go.dev/)
[![Wails](https://img.shields.io/badge/Desktop-Wails%20v2-DF0000)](https://wails.io/)
[![Telegram Bot](https://img.shields.io/badge/🤖%20Telegram-Bot-blue?logo=telegram)](https://t.me/AbitAssistant_bot)
[![Made in Ukraine](https://img.shields.io/badge/Made%20with%20❤️-in%20Ukraine-ffd700)](https://t.me/NeShawyha)

> Реальні шанси на вступ для абітурієнтів України — на основі **живої черги заяв**, а не здогадок. Тягне конкурсні списки з [vstup.osvita.ua](https://vstup.osvita.ua), рахує твій конкурсний бал, показує шанс на бюджет і хто твої справжні конкуренти.

**Desktop-застосунок** на Wails + Svelte (чисте Go всередині). Проходить Cloudflare-захист osvita через справжній браузер у тебе на компʼютері й рахує все **локально** — нічого хостити не треба, профіль нікуди не йде.

---

## 🧠 Як це працює

З 2026 року `vstup.osvita.ua` за Cloudflare Turnstile: прямі запити отримують `403`, а API заяв вимагає одноразовий токен, який видає лише живий браузер. Headless-автоматизацію Cloudflare детектить і блокує.

**Рішення desktop-застосунку:** він відкриває **справжній Chrome** у тебе на машині. Для Cloudflare це звичайний відвідувач, тому перевірка проходить; якщо зʼявляється «я не робот» — ти клікаєш її сам, як на будь-якому сайті. За **один запуск** браузер зчитує і сторінку програми, і всю чергу заяв, після чого:

1. Весь **аналіз рахується локально** (`internal/abit`, `internal/service`) — бал, прохідні, симуляція пріоритетів.
2. Результат кешується в локальний **SQLite** (`~/.config/AbitAssistant/cache.db`).
3. Профіль живе лише в застосунку — нікуди не надсилається.

Перший запит по програмі ~20 секунд (браузеру треба відкритись і пройти перевірку), далі — миттєво з кешу.

## ✨ Можливості

| | |
|---|---|
| 🔎 **Аналіз** | Твоє місце в черзі на програму: бал, шанс на бюджет, список конкурентів із їхніми пріоритетами та розкладкою НМТ. |
| 🎯 **Прогноз** | Розстав програми за пріоритетом — застосунок передбачить, куди саме ти вступиш (рекурсивна симуляція «хто куди піде»). |
| 🧭 **Куди вступлю** | Пошук по галузі та регіону: де твій бал прохідний на бюджет. |
| 💾 **Збережені** | Обрані програми та історія переглядів. |

Темна/світла тема, мобільний веб-режим, усе українською.

## 🚀 Встановлення

### Desktop (готові збірки)

Завантаж бінарник під свою систему зі сторінки [**Releases**](https://github.com/OlexiyOdarchuk/abit-assistant/releases): Windows (`.exe` / інсталятор), macOS (`.dmg`), Linux (`.deb` / `.rpm` / бінарник). Потрібен встановлений **Chrome / Chromium / Edge** (застосунок відкриває його для доступу до osvita).

### Desktop (збірка з коду)

```bash
# потрібні: Go 1.26+, Node 22+, wails CLI, а на Linux — libwebkit2gtk-4.1-dev
go install github.com/wailsapp/wails/v2/cmd/wails@latest

task dev      # запуск у dev-режимі (hot reload)
task build    # зібрати бінарник у build/bin/
```

> **Linux:** збірка йде з тегом `webkit2_41` (див. `Taskfile.yml`) — сучасні дистрибутиви мають `webkit2gtk-4.1`, а не застарілий `4.0`. Тег інертний на Windows/macOS.

## 📐 Архітектура

```
main.go, app.go        Desktop-оболонка (Wails): вікно + байндинги над Core
cmd/osvitacheck/       Валідатор браузерного фетчу проти живої osvita
internal/
  abit/                  Домен: скоринг, аналіз, симуляція (чисте Go)
  apidto/                JSON-контракт між Core і фронтом
  service/               Use-cases (program/discover/simulate/predict)
  desktop/               Core-facade + локальний SQLite-кеш
  parser/
    osvita/                Парсер + інтерфейси фетчерів (403 → браузер)
    osvitabrowser/         chromedp-драйвер: single-shot фетч крізь Turnstile
    abitpoisk/             Крос-довідка заяв абітурієнта
  httpx/                 Rate-limit + circuit breaker до джерел
frontend/              Svelte 5 + Vite (UI, локальні шрифти з кирилицею)
```

`internal/parser/osvitabrowser` — серце обходу Cloudflare: один запуск браузера читає HTML сторінки (`document.documentElement.outerHTML` після розвʼязаної капчі) + пагінує API заяв, реюзаючи щойно-отриманий токен і ретраячи на post-captcha reload. Драйвери реалізують `osvita.ProgramDataFetcher`.

## 🛠️ Розробка

```bash
task check    # go fmt + vet + test
go test ./internal/...
```

Живий тест браузерного фетчу (на десктопі з дисплеєм):

```bash
go run ./cmd/osvitacheck https://vstup.osvita.ua/y2026/r27/41/1612502/
```

## ⚠️ Застереження

Неофіційний інструмент. Дані з `vstup.osvita.ua` та `abit-poisk.org.ua` — фінальне рішення завжди за офіційним реєстром [ЄДЕБО](https://registry.edbo.gov.ua/). Не гати запитами поспіль по багатьох програмах — Cloudflare може почати частіше показувати перевірку.

## 📄 Ліцензія

[GPLv3](LICENSE). Попередня Python-версія — [AbitAssistant_Bot 2.x](https://github.com/OlexiyOdarchuk/AbitAssistant_Bot).
