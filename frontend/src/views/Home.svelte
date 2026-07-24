<script>
  import { profile, lists, history, SUBJECTS } from '../lib/state.svelte.js'
  import Mascot from '../lib/Mascot.svelte'
  import { isDesktop } from '../lib/desktop.js'

  const goAnalyze = (url) => (location.hash = '#/analyze/' + encodeURIComponent(url))

  // short labels for the profile summary chips
  const short = {
    'Українська мова': 'Укр',
    Математика: 'Мат',
    'Історія України': 'Іст',
    'Англійська мова': 'Англ',
    'Українська література': 'Укр.літ',
    Біологія: 'Біо',
    Фізика: 'Фіз',
    Хімія: 'Хім',
    Географія: 'Гео',
    'Інша іноземна': 'Іноз',
  }
  let scores = $derived(
    SUBJECTS.filter((s) => profile.nmt[s] > 0).map((s) => ({ s: short[s] ?? s, v: profile.nmt[s] })),
  )
</script>

<section class="dash">
  <header class="welcome rise">
    <Mascot size={56} />
    <div>
      <h1>Куди далі?</h1>
      <p class="lead">Профіль готовий. Обери, що робимо — однаково корисно.</p>
    </div>
  </header>

  <div class="profile-bar rise">
    <div class="pb-scores">
      {#each scores as sc}
        <span class="pill"><b class="mono">{sc.v}</b> {sc.s}</span>
      {/each}
      {#each profile.quotas as q}<span class="pill soft">{q}</span>{/each}
    </div>
    <a class="edit" href="#/profile">✎ Змінити</a>
  </div>

  <div class="actions">
    <a class="action rise" style="animation-delay:.05s" href="#/analyze">
      <span class="a-icon">🔎</span>
      <h2>Аналіз програми</h2>
      <p>Знаєш, куди хочеш? Встав посилання з vstup.osvita.ua — покажу твій бал, місце і реальних конкурентів.</p>
      <span class="a-go">Аналізувати →</span>
    </a>
    <a class="action rise" style="animation-delay:.12s" href="#/discover">
      <span class="a-icon">🧭</span>
      <h2>Куди я вступлю</h2>
      <p>Ще обираєш? Вкажи галузь і області — підберу бюджетні програми й розсортую за шансом.</p>
      <span class="a-go">Підібрати →</span>
    </a>
  </div>

  <a class="help-cta rise" style="animation-delay:.18s" href="#/help">
    <span class="hc-ic">❓</span>
    <span class="hc-txt">
      <strong>Вперше тут?</strong> За хвилину — як це працює й що означають шанси{#if isDesktop} (і чому відкривається браузер){/if}.
    </span>
    <span class="hc-go">→</span>
  </a>

  {#if history.length}
    <h3>Нещодавні</h3>
    <div class="saved">
      {#each history.slice(0, 5) as h (h.url)}
        <button class="saved-row" onclick={() => goAnalyze(h.url)}>
          <strong>{h.university}</strong>
          <span class="muted">{h.program}</span>
        </button>
      {/each}
    </div>
  {/if}

  {#if lists.length}
    <h3>Збережені</h3>
    <div class="saved">
      {#each lists.slice(0, 5) as l (l.url)}
        <button class="saved-row" onclick={() => goAnalyze(l.url)}>
          <strong>{l.university}</strong>
          <span class="muted">{l.program}</span>
        </button>
      {/each}
    </div>
    {#if lists.length > 5}<a class="more" href="#/lists">Усі збережені ({lists.length}) →</a>{/if}
  {/if}
</section>

<style>
  .welcome { display: flex; align-items: center; gap: 1rem; }
  .welcome h1 { font-size: clamp(1.7rem, 5.5vw, 2.4rem); margin-bottom: 0.2rem; }
  .lead { color: var(--muted); margin: 0; }

  .profile-bar {
    display: flex; justify-content: space-between; align-items: center; gap: 1rem; flex-wrap: wrap;
    background: var(--card); border: 1px solid var(--border); border-radius: 14px;
    padding: 0.7rem 0.9rem; margin: 1.2rem 0 1.8rem; box-shadow: var(--shadow);
  }
  .pb-scores { display: flex; gap: 0.4rem; flex-wrap: wrap; }
  .pill {
    display: inline-flex; gap: 0.3rem; align-items: baseline;
    background: var(--hover); border-radius: 999px; padding: 0.2rem 0.65rem; font-size: 0.85rem;
  }
  .pill b { font-size: 0.95rem; color: var(--accent); }
  .pill.soft { color: var(--accent-ink); background: var(--accent-soft); font-weight: 600; }
  .edit { font-size: 0.9rem; white-space: nowrap; text-decoration: none; }

  .actions { display: grid; grid-template-columns: repeat(auto-fit, minmax(260px, 1fr)); gap: 1rem; }
  .action {
    display: flex; flex-direction: column; text-decoration: none; color: inherit;
    background: var(--card); border: 1px solid var(--border); border-radius: var(--r-card);
    padding: 1.5rem; box-shadow: var(--shadow);
    transition: transform 0.14s, box-shadow 0.14s, border-color 0.14s;
  }
  .action:hover { transform: translateY(-3px); box-shadow: var(--shadow-lift); border-color: color-mix(in srgb, var(--accent) 45%, var(--border)); }
  .a-icon { font-size: 2rem; }
  .action h2 { margin: 0.7rem 0 0.4rem; }
  .action p { margin: 0; color: var(--muted); font-size: 0.93rem; flex: 1; }
  .a-go { margin-top: 1rem; font-weight: 700; color: var(--accent); }

  .help-cta {
    display: flex; align-items: center; gap: 0.8rem; margin-top: 1rem;
    text-decoration: none; color: inherit;
    background: var(--accent-soft); border: 1px solid color-mix(in srgb, var(--accent) 25%, transparent);
    border-radius: var(--r-ctrl); padding: 0.85rem 1rem;
    transition: border-color 0.14s, transform 0.14s;
  }
  .help-cta:hover { border-color: var(--accent); transform: translateY(-1px); }
  .help-cta .hc-ic { font-size: 1.4rem; }
  .help-cta .hc-txt { flex: 1; font-size: 0.92rem; color: var(--text); }
  .help-cta .hc-txt strong { color: var(--accent-ink); }
  .help-cta .hc-go { color: var(--accent); font-weight: 800; font-size: 1.2rem; }

  .saved { display: flex; flex-direction: column; gap: 0.5rem; }
  .saved-row {
    display: flex; flex-direction: column; text-align: left; gap: 0.1rem;
    border: 1px solid var(--border); border-radius: 12px; background: var(--card);
    padding: 0.65rem 0.85rem; cursor: pointer; font: inherit; color: inherit; min-width: 0;
  }
  .saved-row:hover { background: var(--hover); }
  .saved-row strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .muted { color: var(--muted); font-size: 0.85rem; }
  .more { display: inline-block; margin-top: 0.6rem; font-size: 0.9rem; }
</style>
