---
tags: [project/abit-assistant, doc/ux, doc/bot]
---

# 03 — UX і Bot

## Філософія UX

Перевершити Python, а не повторити. У Python мікс reply+inline keyboard'ів, `delete + answer` (мерехтить), in-memory FSM (рестарт = втрата). Тут — навпаки:

1. **All-inline через `/commands`**. Reply-keyboard взагалі не існує.
2. **Edit-in-place** через `renderOrEdit()`. `c.Edit()` коли є callback, `c.Send()` коли немає. Жодного `delete + answer`.
3. **FSM у SQLite** через `internal/bot/fsm`. Юзер посеред введення → перезапуск → continue.
4. **Type-safe callback** через `internal/bot/callback`. Args через `|`-separator, bounds-checked accessors. Не `strings.Split(data, "_")`.
5. **Error middleware** замість `try/except` у кожному handler'і.

Деталі в memory: [[../.claude/memory/feedback_bot_ux.md|feedback-bot-ux]] (у Claude-memory, не комітиться).

## Мапа екранів

```
                                /start (private only)
                                   │
                                   ▼
                    ┌──────────► MENU ◄──────────┐
                    │       /menu, "⬅️ Меню"      │
                    │             │               │
        ┌───────────┼─────────────┼───────────────┼─────────┐
        │           │             │               │         │
        ▼           ▼             ▼               ▼         ▼
   /profile     /search      /lists           /admin    /about
        │           │             │               │
        │           ▼             ▼               ▼
        │       SUMMARY      Список збережених   STATS  /
        │       (бал,        │                   BROADCAST/
        │        шанс,       ├─► Переглянути ───┐ VACUUM
        │        вердикт)   ├─► Оновити (diff) │
        │           │       ├─► Поділитись     │
        │   ┌───────┤       ├─► Експорт JSON   │
        │   ▼       ▼       └─► Видалити       │
        │ Чарт   Список                        │
        │ PNG    (10/page)                     │
        │           │                          │
        │           ▼                          │
        │   Деталі абіт.                       │
        │   ├─► Інші заяви (abit-poisk)        │
        │   ├─► 🔴/🟢 toggle override          │
        │   └─► До списку                      │
        │                                      │
        └─► Підекрани НМТ / Квот / РК / Творчий
```

## Файли

| Файл | Що в ньому |
|---|---|
| `internal/bot/bot.go` | `Bot` struct + Deps + `Run(ctx)` lifecycle. `rootCtx` зберігається для broadcast goroutine. |
| `internal/bot/routes.go` | `registerRoutes()` — реєстрація всіх commands + callback-handlers через map[unique]Handler |
| `internal/bot/middleware.go` | `recoverPanics`, `logUpdates`, `reportErrors`, `trackUser`, `userFacing` |
| `internal/bot/menu.go` | btnUnique* константи, головне меню, `renderOrEdit`, тексти welcome/help/about |
| `internal/bot/handlers.go` | основні handlers: search, summary, list-view, applicant detail/history, toggle-threat |
| `internal/bot/profile.go` | profile flow: subjects, quotas, RegionCoef, creative score, валідація 3+1 |
| `internal/bot/lists.go` | saved-lists CRUD + refresh-diff + share-token + JSON export |
| `internal/bot/admin.go` | `/admin` panel: stats, vacuum, broadcast |
| `internal/bot/callback/` | `Args` parser для callback_data |
| `internal/bot/fsm/` | `Manager` поверх SQLite (`bot_fsm` table) |

## Middleware chain (важливо!)

Реєстрація: `b.tg.Use(logUpdates, reportErrors, trackUser, recoverPanics)`

Telebot ставить LAST у `Use` як INNERMOST. Тобто call ordering на handler:

```
logUpdates(reportErrors(trackUser(recoverPanics(handler))))
```

**Чому recoverPanics — найвнутрішнє:** Якщо panic, defer'ний recover у recoverPanics ловить → returns `errInternal`. Якщо recoverPanics зовнішнє — panic перетинає `reportErrors` до того як стане error → юзер бачить порожній екран.

Це фіксили коммітом `c53fc40`.

## Callback data: type-safe

```go
// Кодування:
kb.Data("📋 До списку", btnUniqueBackToList, callback.Encode(idStr))

// Розкодування:
args := callback.From(c)
id, ok := args.Int(0)
mode := args.String(1)
```

Telegram callback_data обмежений 64 байтами. Тому **не зберігаємо URL у callback** — тримаємо в FSM `state.Data[fsmKeyURL]`, callback несе лише число.

## FSM states

```
search.waiting_url    — після /search без аргументу, чекаємо URL
search.viewing        — переглядаємо результати (URL+page+mode+overrides у data)
profile.enter_score   — введення балу з предмета (current_subject у data)
profile.enter_creative— введення творчого балу
admin.broadcast.text  — admin вводить текст розсилки
admin.broadcast.confirm — підтвердження розсилки (text у data)
```

Persistent через `bot_fsm` table з `ON DELETE CASCADE` на `users`. Видалили юзера → conversation очищається сама.

## Mark applicants як 🔴 / 🟢

Логіка `IsCompetitor` у `pkg/abit/filter.go`. Marker рендериться у `applicantButtonLabel()`. Override на детальному екрані — `handleToggleThreat()` мутує `state.Data["overrides"]` мапу. Поки `search.viewing` стейт активний з тим самим URL — overrides sticky.

## Markdown safety

Telegram парсить legacy Markdown. Якщо у тексті є `]` чи `)` поруч — парсер ламається. Тому:

- `mdEscape()` escape'ить `\ * _ ` [ ]`
- URL не рендеряться через `[text](url)`, а як plain text + `tele.NoPreview` (саме там зловили баг із `)` у query string)

## Private-chat-only

`/search`, `/profile`, `/lists`, `/admin` мають `requirePrivateChat(c)`. У group chat бачив би чужий userScore — це leak.

`handleText` тихо ігнорує у груп — не спамить.

→ [[02 — Доменна модель]] · [[04 — Зберігання]] · [[99 — Технічний борг]]
