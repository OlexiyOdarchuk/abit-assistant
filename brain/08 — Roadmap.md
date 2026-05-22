---
tags: [project/abit-assistant, doc/roadmap]
---

# 08 — Roadmap

## ✅ Зроблено (травень 2026)

- Парсер `vstup.osvita.ua` з fan-out + retry/backoff + warm-up cookies
- Парсер `abit-poisk.org.ua` з broken-cert opt-in
- Декодер усіх форм статусів і квот (port `decoder.py`)
- Profile flow (НМТ + квоти + РК + творчий бал) з валідацією 3+1
- Реальний рейтинговий бал (weighted average + RegionCoef)
- Marker 🔴/🟢 у списку + toggle "Тільки конкуренти"
- Деталі абітурієнта + abit-poisk історія
- Persistent FSM у SQLite
- Saved lists з refresh+diff, share via deep-link token, JSON export
- Summary-екран (місце, шанс, вердикт)
- Manual toggle «це не конкурент» з sticky overrides
- Гістограма балів картинкою (PNG через go-chart/v2)
- `/admin` — статистика, broadcast (rate-limited), vacuum cache
- Docker scratch image + Compose (non-root, read-only rootfs)
- AES-CBC формула EDBO підтверджена з production-коду
- Security + correctness аудит (40+ findings, основні закриті)

## ⏸ Чекає 2026 кампанію

### EDBO драйвер (high priority on launch)

→ [[06 — EDBO Research]]

Потребує живого offer URL з даними. Розрахунковий effort: 1-2 дні роботи після того, як перший XHR captured. Реалізація як `pkg/parser/edbo/`, плагається у ProgramService як другий Source.

Кроки:
1. Capture живого offer → знайти POST endpoint списку
2. Знайти точну формулу salt (зараз знаємо що `"v" + str(a*b)`, треба знайти конкретні a, b)
3. Знайти декілька URL варіантів (2026 vs архів) — щоб бот фоллбекав між осіта та едбо
4. Тести: real-blob-decode case у `internal/edbo/crypto_test.go`

## ⏸ Чекає реальних користувачів

### `/about` показує VERSION

Зараз `cmd/bot/main.go` має `VERSION` build-arg для Docker, але ніде не показано. Колись додати в about-screen.

### Migration tool

Якщо треба rollback міграцій. Зараз `runMigrations` тільки forward. Поки що нічого не ламали — не критично.

### Tests на bot handlers

Поточні bot-тести покривають тільки рідкі речі (NMT subject grid duplicates fix). Більшість handlers зломається не від rev-engineering edge case, а від API-changes у telebot/v3 — а ці changes ловить `go test` через build error.

Якщо колись Bot CI: моки `tele.Context`, перевіряти `c.Send` / `c.Edit` calls.

## 💭 Можливо колись

### Server-frontend

У `AbitAssistant-3.0.md` була ідея Wails-десктоп + сайт-frontend. Це окремий проєкт, не пріоритет. `pkg/abit` уже як бібліотека — будь-хто може взяти.

### CLI-tool публічний

`cmd/cli` зараз dev-тільки. Якщо колись юзер хоче скриптити аналіз свого бал — упакувати як Homebrew/AUR пакет.

### Multi-source enrichment

`EnrichService` готовий, але не використовується. Якщо колись зробити аналітичний режим "покажи мені всю історію 50 топ-абіт" — там і ввімкнути.

### Чарти крім гістограми

Зараз тільки розподіл балів. Можна додати:
- Динаміка кількості заяв у часі (потребує snapshots)
- Heat map спеціальностей по регіонах (агрегувати кілька programs)

### Cross-program compare

«Зберегти 5 програм, порівняти». Saved lists інфраструктура є — додати порівняльну в'ю.

## ❌ НЕ будемо робити

- **Не парсимо vstup.edbo.gov.ua headless-браузером** — це повільно і легко тригериться anti-bot. Тільки прямі XHR через `net/http`.
- **Не зберігаємо PII довше ніж треба** — кеші TTL, saved-lists по share-token (opaque).
- **Не push-сповіщення про зміну балу** — це постійний polling osvita, etyka сумнівна, навантаження зайве.
- **Не платні features** — GPLv3, проект open source.

## Тригери червня

Коли вступна кампанія 2026 стартує (червень-липень):

1. Перевірити що `vstup.osvita.ua` живе → бот працює без жодних змін
2. [[06 — EDBO Research#Action items для червня 2026|EDBO research]] — capture перший живий offer
3. Реалізувати `pkg/parser/edbo/`
4. Перевірити на боях через `aa osvita` і `aa edbo decrypt`
5. Backfill `TestDecryptName_KnownSample` зі справжніми параметрами

→ [[09 — Журнал]] · [[99 — Технічний борг]]
