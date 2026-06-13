<script>
  import { profile, lists, SUBJECTS } from '../lib/state.svelte.js'

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
    <h1>Куди далі?</h1>
    <p class="lead">Профіль готовий. Обери, що робимо — однаково корисно.</p>
  </header>

  <div class="profile-bar rise">
    <div class="pb-scores">
      {#each scores as sc}
        <span class="pill"><b class="mono">{sc.v}</b> {sc.s}</span>
      {/each}
      {#if profile.regionCoef}<span class="pill soft">РК</span>{/if}
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
  .welcome h1 { font-size: clamp(1.8rem, 6vw, 2.6rem); }
  .lead { color: var(--muted); margin-top: -0.3rem; }

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
