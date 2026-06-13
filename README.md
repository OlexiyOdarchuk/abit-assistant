# AbitAssistant 3.0

[![License: GPLv3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0.html)
[![Go 1.26+](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](https://go.dev/)
[![Telegram Bot](https://img.shields.io/badge/🤖%20Telegram-Bot-blue?logo=telegram)](https://t.me/AbitAssistant_bot)
[![Made in Ukraine](https://img.shields.io/badge/Made%20with%20❤️-in%20Ukraine-ffd700)](https://t.me/NeShawyha)

> Бот для абітурієнтів України. Тягне конкурсні списки з [vstup.osvita.ua](https://vstup.osvita.ua), показує реальних конкурентів на бюджет, рахує твій рейтинговий бал.

## 🔁 Версії

Це Go-переписування Python-бота [**AbitAssistant_Bot 2.x**](https://github.com/OlexiyOdarchuk/AbitAssistant_Bot) (попередня версія, переходить в архів). Ідея та сама — реалізація нова:

| | Python 2.x | Go 3.0 |
|---|---|---|
| Архітектура | моноліт `app/` | Standard Go Layout (`cmd/internal/pkg`) |
| SQLite | n/a (PostgreSQL) | pure-Go `modernc.org/sqlite`, без CGo |
| FSM | aiogram, in-memory | Persistent у SQLite — переживає рестарт |
| Callback handling | `data.split("_")` | Typed args + `Btn.Unique` |
| UX | reply + inline keyboards mix | all-inline, edit-in-place |
| Тести | — | 70+ unit-тестів |
| Docker | python:slim ~400MB | scratch ~30MB (план) |
| Конкурент-фільтр | sync + Selenium fallback | concurrent-safe, готовий до enrich через abit-poisk |

## 📐 Архітектура

```
cmd/                  Точки входу (тільки wiring)
  bot/                  Telegram бот
  cli/                  Dev CLI

pkg/                  Публічне ядро — підключається через `go get`
  abit/                 Доменна модель: типи, decoder, filter, stats, calculator, links
  parser/               Інтерфейс Source + реалізації
    osvita/               vstup.osvita.ua (JSON API + JS-vars зі сторінки)
    abitpoisk/            abit-poisk.org.ua (пошук конкретного абітурієнта)

internal/             Приватна реалізація
  bot/                  Telegram handlers, FSM, callback codec, middleware
    callback/             Typed args для callback_data
    fsm/                  Persistent FSM поверх SQLite
  service/              Use cases: ProgramService, ApplicantService, EnrichService
  storage/              SQLite + sqlc-generated queries + embed migrations
    migrations/           SQL schema
    queries/              SQL queries (sqlc input)
    db/                   sqlc output (не редагувати)
  config/               env loader + .env
  edbo/                 AES-CBC дешифрування ПІБ з vstup.edbo.gov.ua (експериментально)
```

## 📦 Як бібліотека

Все під `pkg/` — public API. Імпортується з будь-якого Go-проекту:

```go
import (
    "context"
    "github.com/OlexiyOdarchuk/abit-assistant/pkg/abit"
    "github.com/OlexiyOdarchuk/abit-assistant/pkg/parser/osvita"
    "github.com/OlexiyOdarchuk/abit-assistant/pkg/parser/abitpoisk"
)

ctx := context.Background()

// 1. Парсимо програму
p := osvita.New()
prog, err := p.Parse(ctx, "https://vstup.osvita.ua/y2025/r14/282/1471029/")

// 2. Декодуємо у []Abiturient (статуси, квоти, коеф, calc-link, abit-link)
abits := abit.Decode(prog)

// 3. Рахуємо власний рейтинговий бал
rating := abit.ComputeRating(prog, abit.RatingInput{
    NMT: map[string]float64{
        "Українська мова":  180,
        "Математика":       170,
        "Історія України":  175,
        "Англійська мова":  190,
    },
    RegionCoef: true,
})

// 4. Фільтр + статистика
top := abit.Filter{
    StatusInclude: []string{"До наказу", "Рекомендовано"},
    Funding:       abit.FundingBudget,
    PriorityMax:   1,
}.Apply(abits)

summary := abit.Summarize(abits)
hist    := abit.Distribution(abits, 5)
real    := abit.RealCompetitors(abits, rating, true)

// 5. abit-poisk lookup
c := abitpoisk.New(abitpoisk.WithInsecureTLS())
entries, _ := c.Search(ctx, "Бовкун О В")
```

Усі функції в `pkg/abit/` pure — без I/O. Парсери ходять у мережу, але приймають `context.Context` і безпечні до cancel.

## 🚀 Запуск бота

```bash
cp .env.example .env
# відредагуй .env: TELEGRAM_TOKEN, опц. ADMIN_IDS
go run ./cmd/bot
```

Production-білд (scratch-friendly):
```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o aa-bot ./cmd/bot
```

### Через Docker
```bash
cp .env.example .env       # заповни TELEGRAM_TOKEN
mkdir -p data && chmod 777 data    # тому що контейнер працює як UID 65532
docker compose up -d
docker compose logs -f
```
Образ — scratch-based, **~13 МБ**. SQLite-файл і WAL живуть у томі `./data` (під WAL ~700 КБ на 2k заявок).

Контейнер закручено: `cap_drop: ALL`, `read_only: true`, `no-new-privileges`, USER 65532. Telegram через long-polling — порти назовні не потрібні.

### Команди в чаті

| Команда | Що робить |
|---|---|
| `/start`, `/menu` | Головне меню |
| `/profile` | НМТ, квоти, РК, творчий бал |
| `/search <url>` | Аналіз програми; можна просто кинути посилання |
| `/lists` | Збережені аналізи: refresh+diff, share через deep-link, експорт JSON |
| `/help`, `/about` | Довідка |
| `/cancel` | Вийти з поточного діалогу |

## 🧪 Dev CLI

```bash
go run ./cmd/cli osvita    https://vstup.osvita.ua/y2025/r14/282/1471029/
go run ./cmd/cli abitpoisk "Бовкун О В"
go run ./cmd/cli osvita ... | go run ./cmd/cli decode
go run ./cmd/cli edbo decrypt <base64> <n> <prsid> [year]
```

## 🛠 Розробка

```bash
go test ./...                          # тести
go vet ./...                           # vet
~/go/bin/sqlc generate                 # після правки SQL
```

Database file за замовчуванням `./data/abit.db` — переоприділюється через `DATABASE_PATH`.

## ⚙️ Стек

- **Go 1.26+**
- **[telebot v3](https://gopkg.in/telebot.v3)** — Telegram Bot API
- **[modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)** — pure-Go SQLite
- **[sqlc](https://github.com/sqlc-dev/sqlc)** — типобезпечні запити з SQL
- **[goquery](https://github.com/PuerkitoBio/goquery)** + `golang.org/x/net/html` — HTML scraping
- **[go-chart/v2](https://github.com/wcharczuk/go-chart)** — pure-Go PNG charts

## 🗺 Roadmap

- [x] Парсер `vstup.osvita.ua` (fan-out + retry/backoff + cookie warm-up)
- [x] Парсер `abit-poisk.org.ua` (broken-cert fallback)
- [x] Декодер з усіма формами статусів і квот
- [x] Profile flow (НМТ + квоти + РК + творчий) з валідацією 3+1
- [x] Реальний рейтинговий бал (weighted average + RK)
- [x] Marker 🔴/🟢 у списку + toggle «Тільки конкуренти»
- [x] Деталі абітурієнта + abit-poisk історія
- [x] Persistent FSM у SQLite
- [x] Saved lists з refresh+diff, share via deep-link, JSON export
- [x] Summary-екран (місце, шанс, вердикт) після аналізу
- [x] Manual toggle «це не конкурент» з sticky overrides
- [x] Гістограма балів картинкою (PNG через go-chart/v2)
- [x] `/admin` — статистика, broadcast (rate-limited), vacuum cache
- [x] Docker scratch image + Compose (non-root, read-only rootfs)
- [ ] Повноцінний EDBO драйвер (`vstup.edbo.gov.ua`) — потребує живого reverse-engineering

## 📄 Ліцензія

[GPLv3](https://www.gnu.org/licenses/gpl-3.0.html) — як у [попередньої версії](https://github.com/OlexiyOdarchuk/AbitAssistant_Bot).

## 💸 Підтримати

Сервери, кава, мрія про новий ноут — все на одній банці: [Monobank](https://send.monobank.ua/jar/23E3WYNesG).

## 👤 Автор

Олексій Одарчук — [@NeShawyha](https://t.me/NeShawyha) · [GitHub](https://github.com/OlexiyOdarchuk)
