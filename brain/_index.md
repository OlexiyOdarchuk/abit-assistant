---
tags: [project/abit-assistant, vault/home]
---

# 🧠 AbitAssistant Brain

Це personal-knowledge-vault про проєкт **abit-assistant** — Go-переписування Python-бота AbitAssistant. Не README. README — це для нових людей. Brain — для **майбутнього себе**, який повертається до коду через місяць-два і питає «а чому так?».

Все що тут — лежить поряд із кодом, у Obsidian-форматі. Wikilinks `[[…]]` між нотатками працюють і в Obsidian, і в GitHub preview.

## 🗂 Карта vault'а

- [[00 — Огляд]] — TL;DR проєкту, мотивація, як ми тут опинилися
- [[01 — Архітектура]] — Standard Go Layout, межі шарів, dependency rules
- [[02 — Доменна модель]] — Abiturient, Program, Analysis, ChanceLevel, що означає
- [[03 — UX і Bot]] — all-inline, edit-in-place, FSM у SQLite, callback codec, мапа екранів
- [[04 — Зберігання]] — SQLite + sqlc, схеми, міграції, JSON-helpers, шлях БД
- [[05 — Парсери і сервіси]] — osvita, abitpoisk, edbo, ProgramService, EnrichService, singleflight
- [[06 — EDBO Research]] — що знайшли, AES-формула, чекати кампанію 2026
- [[07 — Розробка]] — як запустити, тести, Docker, dev CLI, env
- [[08 — Roadmap]] — короткостроковий + довгостроковий план
- [[09 — Журнал]] — кратко по комітах, у зворотньому порядку
- [[10 — Правила вступу і формули]] — офіційні КБ/РК/ГК/квоти/статуси 2025-2026 + джерела (еталон логіки)
- [[99 — Технічний борг]] — known issues, не-зроблене з аудиту, нотатки на майбутнє

## 📍 Швидкий вхід для повернення в проєкт

1. Прочитати [[00 — Огляд]] — нагадає що це за звір.
2. [[09 — Журнал]] — що було зроблено останнє.
3. [[08 — Roadmap]] — що далі.
4. [[06 — EDBO Research]] — якщо це червень+ і вступна кампанія стартувала.

## 🛠 Якщо просто треба запустити

→ [[07 — Розробка]] · `docker compose up -d` · все.

## ⏸ Стан станом на 2026-05-23 (момент закриття до червня)

- ✅ Все що було в Python v2 — реалізовано на Go з покращеннями
- ✅ Refactor-аудит (security + correctness) — основні findings закриті
- ✅ AES-формула EDBO підтверджена з production-коду через Playwright capture
- ⏸ Live EDBO драйвер чекає 2026 кампанію (архіви порожні)
- ⏸ Користувач: 1 (я). Бот працює, тести зелені.
