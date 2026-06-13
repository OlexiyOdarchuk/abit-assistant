<script>
  import { lists, removeList } from '../lib/state.svelte.js'
  const goAnalyze = (url) => (location.hash = '#/analyze/' + encodeURIComponent(url))
</script>

<section>
  <h1>Збережені</h1>
  <p class="lead">Програми, які ти зберіг. Зберігаються у твоєму браузері.</p>

  {#if lists.length === 0}
    <p class="muted">Поки порожньо. Збережи програму кнопкою «💾» на екрані аналізу.</p>
  {:else}
    <div class="rows">
      {#each lists as l (l.url)}
        <div class="row">
          <button class="open" onclick={() => goAnalyze(l.url)}>
            <strong>{l.university}</strong>
            <span class="muted">{l.program}</span>
          </button>
          <button class="rm" title="Прибрати" onclick={() => removeList(l.url)}>🗑</button>
        </div>
      {/each}
    </div>
  {/if}
</section>

<style>
  .lead { color: var(--muted); margin-top: -0.5rem; }
  .muted { color: var(--muted); }
  .rows { display: flex; flex-direction: column; gap: 0.5rem; }
  .row { display: flex; gap: 0.5rem; align-items: stretch; }
  .open {
    flex: 1;
    display: flex;
    flex-direction: column;
    text-align: left;
    padding: 0.7rem 0.9rem;
    border: 1px solid var(--border);
    border-radius: 12px;
    background: var(--card);
    cursor: pointer;
    font: inherit;
    color: inherit;
    min-width: 0;
  }
  .open:hover { background: var(--hover); }
  .open strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .rm {
    border: 1px solid var(--border);
    background: var(--card);
    border-radius: 12px;
    padding: 0 0.9rem;
    cursor: pointer;
  }
</style>
