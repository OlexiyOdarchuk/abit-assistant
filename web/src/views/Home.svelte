<script>
  import { profileFilled } from '../lib/state.svelte.js'

  const features = [
    { icon: '🎯', t: 'Реальні конкуренти', d: 'Не весь список, а ті, хто справді бореться з тобою за бюджетне місце — за балом, статусом і поданими оригіналами.' },
    { icon: '🧮', t: 'Шанси за алгоритмом', d: 'Рахуємо за логікою адресного розміщення: пріоритети, квоти, вільні місця після зарахованих.' },
    { icon: '🧭', t: 'Куди я вступлю', d: 'Обери галузь і області — підберемо бюджетні програми й розсортуємо за шансом: надійні / на межі / амбіційні.' },
    { icon: '🔒', t: 'Без акаунтів', d: 'Бали НМТ живуть лише у твоєму браузері. Нічого не зберігаємо на сервері.' },
  ]
  const steps = [
    { n: '1', t: 'Заповни профіль', d: 'Бали НМТ, квоти, регіональний коефіцієнт.' },
    { n: '2', t: 'Встав посилання або обери галузь', d: 'Програма з vstup.osvita.ua — або пошук за галуззю й областю.' },
    { n: '3', t: 'Побач свої шанси', d: 'Конкурсний бал, місце, шанс на бюджет і хто реально конкурує.' },
  ]
</script>

<section class="hero grid-bg">
  <div class="hero-inner rise">
    <span class="badge">vstup.osvita.ua · твій шанс на бюджет</span>
    <h1>Вступай на бюджет<br /><span class="gradient-text">з холодним розрахунком</span></h1>
    <p class="lead">
      Встав посилання на програму — і побачиш свій конкурсний бал, реальних конкурентів і шанси на
      бюджет. Або підбери, куди вступати, за галуззю та областю.
    </p>
    <div class="cta">
      <a class="btn-primary" href="#/analyze">
        Аналіз програми
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M14 5l7 7m0 0l-7 7m7-7H3" /></svg>
      </a>
      <a class="btn-ghost" href="#/discover">🧭 Куди я вступлю</a>
    </div>
    {#if !profileFilled()}
      <p class="tip">Почни з <a href="#/profile">профілю</a> — без балів НМТ шанси не порахувати.</p>
    {/if}
  </div>
</section>

<section class="band">
  <div class="features">
    {#each features as f, i}
      <article class="feature rise" style="animation-delay: {0.05 * i + 0.1}s">
        <span class="f-icon">{f.icon}</span>
        <h2>{f.t}</h2>
        <p>{f.d}</p>
      </article>
    {/each}
  </div>
</section>

<section class="band steps-band">
  <h3>Як це працює</h3>
  <div class="steps">
    {#each steps as s}
      <div class="step">
        <span class="s-num mono">{s.n}</span>
        <div>
          <strong>{s.t}</strong>
          <p>{s.d}</p>
        </div>
      </div>
    {/each}
  </div>
</section>

<style>
  .hero {
    position: relative;
    border-bottom: 1px solid var(--border);
    background: linear-gradient(to bottom, var(--bg), color-mix(in srgb, var(--card) 60%, var(--bg)));
    margin: -1.5rem -1.5rem 0;
    padding: clamp(2.5rem, 9vw, 5rem) 1.5rem;
  }
  .hero-inner { max-width: 640px; margin: 0 auto; text-align: center; }
  .badge {
    display: inline-block;
    border: 1px solid color-mix(in srgb, var(--accent) 35%, transparent);
    background: var(--accent-soft);
    color: var(--accent-ink);
    padding: 0.35rem 0.9rem;
    border-radius: 999px;
    font-size: 0.72rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  .hero h1 { margin: 1.1rem 0 0; font-size: clamp(2rem, 8vw, 3.3rem); font-weight: 800; }
  .lead { color: var(--muted); font-size: 1.08rem; margin: 1.1rem auto 0; max-width: 540px; }
  .cta { display: flex; gap: 0.8rem; justify-content: center; flex-wrap: wrap; margin-top: 1.8rem; }
  .btn-primary {
    display: inline-flex; align-items: center; gap: 0.5rem;
    background: var(--accent); color: #fff; text-decoration: none;
    padding: 0.8rem 1.5rem; border-radius: var(--r-ctrl); font-weight: 700;
    transition: transform 0.12s, box-shadow 0.12s, filter 0.12s;
  }
  .btn-primary:hover { transform: translateY(-2px); box-shadow: var(--glow); filter: brightness(1.06); }
  .btn-primary svg { width: 1rem; height: 1rem; transition: transform 0.12s; }
  .btn-primary:hover svg { transform: translateX(3px); }
  :root.dark .btn-primary { color: #07101f; }
  .btn-ghost {
    display: inline-flex; align-items: center;
    border: 1.5px solid var(--border); background: var(--card); color: var(--text);
    text-decoration: none; padding: 0.8rem 1.4rem; border-radius: var(--r-ctrl); font-weight: 600;
    transition: transform 0.12s, border-color 0.12s;
  }
  .btn-ghost:hover { transform: translateY(-2px); border-color: var(--accent); }
  .tip { color: var(--muted); font-size: 0.9rem; margin-top: 1.2rem; }

  .band { max-width: 760px; margin: 0 auto; padding: 2.2rem 0 0; }
  .features { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 0.9rem; }
  .feature {
    background: var(--card); border: 1px solid var(--border); border-radius: var(--r-card);
    padding: 1.2rem; box-shadow: var(--shadow);
  }
  .f-icon { font-size: 1.6rem; }
  .feature h2 { margin: 0.6rem 0 0.3rem; font-size: 1.05rem; }
  .feature p { margin: 0; color: var(--muted); font-size: 0.92rem; }

  .steps { display: flex; flex-direction: column; gap: 0.8rem; }
  .step { display: flex; gap: 0.9rem; align-items: flex-start; }
  .s-num {
    flex: none; width: 2.1rem; height: 2.1rem; display: grid; place-items: center;
    background: var(--accent-soft); color: var(--accent-ink); border-radius: 10px;
    font-weight: 700; font-size: 1.05rem;
  }
  .step strong { display: block; }
  .step p { margin: 0.1rem 0 0; color: var(--muted); font-size: 0.92rem; }
</style>
