<script>
  // Static guide: what the app does, how to use it, how it works under the
  // hood, and when it's useful. Pure content — no backend calls.
  import { isDesktop } from '../lib/desktop.js'

  const steps = [
    {
      n: '1',
      t: 'Заповни профіль',
      d: 'Введи свої бали НМТ (3 обовʼязкові предмети + 1 на вибір) і квоти, якщо є. Це рахує твій конкурсний бал. Профіль зберігається локально — вводиш один раз.',
    },
    {
      n: '2',
      t: 'Встав посилання на програму',
      d: 'Скопіюй адресу освітньої програми з vstup.osvita.ua (напр. .../y2026/r27/41/1612502/) у вкладці «Аналіз».',
    },
    {
      n: '3',
      t: 'Пройди перевірку та дивись результат',
      d: isDesktop
        ? 'Відкриється вікно браузера — якщо Cloudflare покаже «я не робот», клікни галочку. Далі застосунок сам зчитає чергу заяв і покаже твій бал, шанси на бюджет і хто твої реальні конкуренти.'
        : 'Застосунок зчитає чергу заяв і покаже твій бал, шанси на бюджет і хто твої реальні конкуренти.',
    },
  ]

  const features = [
    { i: '🔎', t: 'Аналіз', d: 'Твоє місце в черзі на конкретну програму: бал, шанс на бюджет, список конкурентів із їхніми пріоритетами.' },
    { i: '🎯', t: 'Прогноз', d: 'Постав програми у пріоритетному порядку — застосунок передбачить, куди саме ти вступиш.' },
    { i: '🧭', t: 'Куди вступлю', d: 'Пошук по галузі та регіону: де саме твій бал прохідний на бюджет.' },
    { i: '💾', t: 'Збережені', d: 'Обрані програми та історія переглядів — усе під рукою.' },
  ]
</script>

<div class="help rise">
  <h1>Як користуватися</h1>
  <p class="lead">
    AbitAssistant показує твої <strong>реальні шанси на вступ</strong> — на основі живої черги
    заяв абітурієнтів, а не здогадок.
  </p>

  <h3>За 3 кроки</h3>
  <div class="steps">
    {#each steps as s}
      <div class="step card">
        <span class="num">{s.n}</span>
        <div>
          <strong>{s.t}</strong>
          <p>{s.d}</p>
        </div>
      </div>
    {/each}
  </div>

  <h3>Що вміє</h3>
  <div class="features">
    {#each features as f}
      <div class="feat card">
        <span class="ic">{f.i}</span>
        <div><strong>{f.t}</strong><p>{f.d}</p></div>
      </div>
    {/each}
  </div>

  <h3>Як це працює</h3>
  <div class="card how">
    <p>
      Сайт vstup.osvita.ua з 2026 року захищений Cloudflare — звичайні боти він блокує.
      {#if isDesktop}
        Тому застосунок відкриває <strong>справжній браузер</strong> просто у тебе на компʼютері:
        для Cloudflare це звичайний відвідувач, тому перевірка проходить. Якщо зʼявляється
        галочка «я не робот» — ти клікаєш її сам, як на будь-якому сайті.
      {:else}
        Дані підтягуються через сервер.
      {/if}
    </p>
    <p>
      Далі <strong>весь аналіз рахується локально</strong>, у тебе на пристрої: бал, прохідні,
      симуляція пріоритетів. Твій профіль нікуди не надсилається — він живе лише тут.
    </p>
    <ul>
      <li><strong>Перший запит ~20 секунд</strong> — стільки треба браузеру відкритись і пройти перевірку. Далі результат кешується.</li>
      <li><strong>Не «клацай» надто часто</strong> поспіль по багатьох програмах — Cloudflare може почати частіше показувати галочку. Дай собі паузи.</li>
    </ul>
  </div>

  <h3>Коли це треба</h3>
  <div class="card">
    <p>
      Під час вступної кампанії, коли треба <strong>тверезо оцінити шанси</strong>: чи проходиш на
      бюджет, наскільки щільна черга, кого реально обійти, і як розставити пріоритети, щоб не
      втратити місце. Особливо корисно в останні дні подачі, коли черга змінюється щогодини.
    </p>
  </div>

  <p class="foot">
    Дані: <a href="https://vstup.osvita.ua" target="_blank" rel="noreferrer">vstup.osvita.ua</a>,
    <a href="https://abit-poisk.org.ua" target="_blank" rel="noreferrer">abit-poisk.org.ua</a>.
    Це неофіційний інструмент — фінальне рішення завжди за офіційними даними ЄДЕБО.
  </p>
</div>

<style>
  .lead { font-size: 1.05rem; color: var(--muted); margin: 0 0 0.5rem; }
  .steps { display: grid; gap: 0.7rem; }
  .step { display: flex; gap: 0.9rem; align-items: flex-start; margin: 0; }
  .num {
    flex: none;
    width: 2rem;
    height: 2rem;
    border-radius: 50%;
    display: grid;
    place-items: center;
    background: var(--accent-soft);
    color: var(--accent);
    font-weight: 800;
    font-family: var(--font-display);
  }
  .step p, .feat p, .how p { margin: 0.25rem 0 0; color: var(--muted); font-size: 0.92rem; }
  .features { display: grid; grid-template-columns: repeat(auto-fit, minmax(230px, 1fr)); gap: 0.7rem; }
  .feat { display: flex; gap: 0.8rem; align-items: flex-start; margin: 0; }
  .feat .ic { font-size: 1.5rem; line-height: 1; }
  .how ul { margin: 0.6rem 0 0; padding-left: 1.1rem; color: var(--muted); font-size: 0.92rem; }
  .how li { margin: 0.35rem 0; }
  .how li strong { color: var(--text); }
  .foot { color: var(--muted); font-size: 0.85rem; margin-top: 1.4rem; }
</style>
