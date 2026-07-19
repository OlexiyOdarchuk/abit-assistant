---
tags: [project/abit-assistant, doc/edbo, status/reversed]
---

# 06 — EDBO Research

> **ОНОВЛЕНО 11.06.2026 — API повністю реверснуто вживу.** Повна карта
> ендпоінтів, схем і виправлена формула → [[../tools/edbo-reverse/API.md|tools/edbo-reverse/API.md]].
> Нижче — історичний контекст; ключові висновки вже не «blocked».

> **⚠️ ОНОВЛЕНО 19.07.2026 — кампанія 2026 стартувала, EDBO змінив портал.**
> `vstup.edbo.gov.ua` тепер **Next.js SPA** (`/_next/static/chunks/...`), а не
> jQuery/Handlebars 2025-сторінка. Наслідки:
> - Піддомен `vstup2026.edbo.gov.ua` **не існує** (DNS не резолвиться); поточний
>   портал — просто `vstup.edbo.gov.ua`. Старий `vstup2025.edbo.gov.ua` (з
>   `functions.js`, `dec`/`multiply`) ще живий як архів.
> - Реверснутий 2025-API (`/offers-list/`, `/offer-requests/`, Handlebars-
>   темплейти) — **для 2026 не діє**; Next.js тягне дані з іншого бекенду.
> - Крипта `internal/edbo/crypto.go` лишається валідною лише для 2025-архіву.
>   Чи 2026 Next.js досі AES-шифрує ПІБ тим самим ключем — **НЕ перевірено**
>   (стара доставка зникла). Потрібен свіжий `capture.py` проти нового API.
> - **Це не блокер:** osvita вже працює на 2026 (парсер обсягів полагоджено
>   19.07 — новий inline-формат `Максимальне держзамовлення: <b>N</b>`).
>   EDBO — резервне джерело, не критичне зараз.
> - Дія, якщо колись беремось: `capture.py --interactive` проти живого
>   `vstup.edbo.gov.ua/offer/<id>/` → знайти JSON-endpoint зі заявами → перевірити,
>   чи ПІБ шифровані і яким ключем → тоді `internal/parser/edbo/`.

## TL;DR

ЄДЕБО — першоджерело даних (osvita.ua тільки проксіює). Раніше парсер
не писали бо архіви офер-сторінок чистяться після кампанії. **Станом на
11.06.2026 `vstup2025.edbo.gov.ua` знову віддає живі дані**, і весь
ланцюг реверснуто: `/university-search/` → `/offers-universities/` →
`/offers-list/` → `/offer/<id>/` → `POST /offer-requests/`. POST-и
вимагають заголовок `Origin` (інакше тихо порожньо). `/offers-list/`
віддає обсяги+коефіцієнти+статистику JSON-ом (HTML-скрейп osvita майже
не потрібен). `/offer-requests/` віддає рейтинг заяв із відкритим `kv`.

### ⚠️ Виправлення формули salt

Стара нотатка нижче казала, що `(7500-prsid)*n` — хибний guess, і що
правильно `'v'+a*b`. **Це було неправильно.** Живий шаблон офера:
`{{dec fio (multiply (subtract 7500 prsid) n)}}` → **salt = "v" +
((7500 - prsid) * n)**. Підтверджено на 85 рядках (усі `p` →
"<пріоритет> (Б)"). Код виправлено: `crypto.go::SaltName/DecryptName`,
доданий реальний regression-тест `TestDecryptName_KnownSample`.

## Що знайшли

### 1. AES-CBC формула (verbatim з functions.js)

```js
Handlebars.registerHelper('dec', function(a, b) {
    const $sk = b;
    const $si = '2025';
    const k = CryptoJS.SHA256($sk).toString(CryptoJS.enc.Hex).substring(0, 32);
    let   i = CryptoJS.SHA256($si).toString(CryptoJS.enc.Hex).substring(0, 16);
    const e = CryptoJS.enc.Base64.parse(a).toString(CryptoJS.enc.Utf8);
    const d = CryptoJS.AES.decrypt(e, Utf8.parse(k), { iv: Utf8.parse(i) }).toString(Utf8);
    return d;
});

Handlebars.registerHelper('multiply', function(a, b) {
    return 'v' + (Number(a) * Number(b));
});
```

Перекладено в `internal/edbo/crypto.go`:

```go
salt = "v" + str(a*b)              // SaltMultiply(a, b)
key  = SHA256(salt).Hex[:32]       // 32 ASCII bytes
iv   = SHA256(year).Hex[:16]       // 16 ASCII bytes
plain = AES-CBC-Decrypt(base64(base64(blob)), key, iv)
```

Стара формула `(7500-prsid)*n` була guess з раннього main.go експерименту, не з продакшен-коду. Знайшли і виправили в коммі `9b9f58b`.

### 2. AJAX endpoints (з `js/offers_search_form.js`)

```
POST /university-search/?ns=<name_substring>
POST /offers-universities/      ← запит зі списком фільтрів
POST /offers-list/              ← {ids: [...]} → JSON список offers
```

Усі повертають JSON. На офер-сторінках — handlebars-templates, що рендеряться client-side з даних, які приходять через окремий fetch (точно якого ще не знаємо — потрібен живий offer).

### 3. JS глобальні мапи (functions.js)

```js
indicators = {
    'ob': 'Бал за особливі успіхи',
    'gk': 'Галузевий коефіцієнт',
    'sk': 'Сільський коефіцієнт',
    'pk': 'Першочерговий коефіцієнт',
    'rk': 'Регіональний коефіцієнт'
}

stats_fields = {
    't': 'Подано заяв',
    'a': 'Допущено до конкурсу',
    'b': 'Заяв на бюджет',
    'ka': 'Сер. бал', 'km': 'Мін. бал', 'kx': 'Макс. бал',
    'r': 'Рекомендовано на загальних підставах',
    'ob': 'Зараховано на бюджет',
    'oc': 'Зараховано на контракт',
    'rm': 'Мін. бал рекомендованих',
    'obm': 'Мін. бал зарахованих на бюджет',
    'ocm': 'Мін. бал зарахованих на контракт'
}
```

Короткі ключі — це формат JSON-payload з API. Decoder, коли буде писатись, конвертуватиме `{"t": 2278, "ka": 168.05, ...}` у `Analysis`.

## Чому не зробили

- Усі offer URLs віддають skeleton "Конкурсна пропозиція не знайдена" (10 КБ HTML без даних)
- web.archive.org snapshots на vstup2025/offer/<id>/ — порожньо
- Поточний vstup.edbo.gov.ua/offer/<id>/ — 308 redirect (2026 портал ще не запущено повністю)
- Без живого XHR із даними неможливо знайти точний format payload, точні параметри `multiply(a, b)`, де саме `prsid` і `n`

## Інструменти готові

`tools/edbo-reverse/` (gitignored):

- `capture.py` — Playwright headless/interactive, ловить ВСІ XHR + inline scripts + cookies + screenshot
- `analyze.py` — постпроцесор: hosts, endpoints, JSON schemas, JS-vars, base64-patterns

Коли стартує 2026:
```bash
cd tools/edbo-reverse
./.venv/bin/python capture.py https://vstup.edbo.gov.ua/offer/<живий_id>/ --interactive
./.venv/bin/python analyze.py out/<...>.json
```

Дамп показує які XHR летять, чим вони відповідають, які поля зашифровані. Тоді — годину роботи на `internal/parser/edbo/` і готово.

## Action items для червня 2026

1. **Перевірити кампанію** — vstup.edbo.gov.ua заполниться даними у червні-липні.
2. **Знайти offer ID** — будь-який живий URL з offers-list.
3. **Запустити `capture.py --interactive`** — клацнути на сторінці, дозволити підвантажити заяви.
4. **Знайти POST endpoint** який віддає список абітурієнтів.
5. **Знайти salt format** — у Handlebars template буде видно конкретно `multiply(X, Y)` де X/Y — це поля з payload.
6. Реалізувати `internal/parser/edbo/` як `Source` interface.
7. Wire через ProgramService як fallback / second source.

## Файли в коді

- `internal/edbo/crypto.go` — `Decrypt(blob, salt, year)`, `SaltMultiply(a, b)`, `DecryptName(blob, prsid, n, year)`, `Encrypt(...)` для тестів
- `internal/edbo/crypto_test.go` — round-trip 5 кейсів, salt-multiply pin, wrong-salt rejection, invalid-base64

`TestDecryptName_KnownSample` залишений як `t.Skip(...)` — re-enable коли матимемо `(blob, prsid, n, year, expected_name)` quadruple.

## Лінки

- [[../tools/edbo-reverse/README.md|edbo-reverse README]] (поза vault'ом, але корисно)
- [[09 — Журнал|Журнал]] · коміт `9b9f58b` — фікс формули
- [[08 — Roadmap|Roadmap]] · "EDBO драйвер" у beyond-2026 секції
