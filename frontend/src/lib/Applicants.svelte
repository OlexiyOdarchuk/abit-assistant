<script>
  import { applicant as fetchApplicant } from './api.js'

  let { applicants = [] } = $props()

  let onlyCompetitors = $state(false)
  let page = $state(0)
  const pageSize = 25

  // Competitor tiers (abit.CompetitorTier): 3 real 🔴, 2 potential 🟡,
  // 1 unlikely ⚪ (priority 3+), 0 none. "Лише конкуренти" keeps real + potential.
  const TIER = { real: 3, potential: 2, unlikely: 1 }
  const tierMark = (t) => (t === TIER.real ? '🔴' : t === TIER.potential ? '🟡' : t === TIER.unlikely ? '⚪' : '')

  let view = $derived(onlyCompetitors ? applicants.filter((a) => a.tier >= TIER.potential) : applicants)
  let maxPage = $derived(Math.max(0, Math.ceil(view.length / pageSize) - 1))
  let pageRows = $derived(view.slice(page * pageSize, page * pageSize + pageSize))

  $effect(() => {
    // keep page in range when the filter shrinks the list
    if (page > maxPage) page = maxPage
  })

  // applicant history modal
  let openRow = $state(null)
  let history = $state(null)
  let histConfident = $state(false)
  let histErr = $state('')
  let histLoading = $state(false)

  const masked = (name) => name.includes('###')

  async function openHistory(row) {
    openRow = row
    history = null
    histErr = ''
    if (masked(row.name)) {
      histErr = 'Ім’я приховане — інші заяви недоступні'
      return
    }
    histLoading = true
    try {
      const resp = await fetchApplicant(row.name, row.score)
      history = resp.entries ?? []
      histConfident = resp.confident ?? false
    } catch (e) {
      histErr = e.message
    } finally {
      histLoading = false
    }
  }
  const closeHistory = () => (openRow = null)

  // Escape closes the modal — keyboard users shouldn't get stuck on it.
  $effect(() => {
    if (!openRow) return
    const onKey = (e) => e.key === 'Escape' && closeHistory()
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })
</script>

<div class="bar">
  <label class="toggle">
    <input type="checkbox" bind:checked={onlyCompetitors} />
    Лише конкуренти
  </label>
  <span class="count">{view.length} заяв</span>
</div>

<p class="legend">
  🔴 реальний (пріоритет 1) · 🟡 потенційний (пріоритет 2) ·
  ⚪ пріоритет 3+ — майже напевно піде вище
</p>

<div class="rows">
  {#each pageRows as a (a.id)}
    <button class="row" class:competitor={a.tier === TIER.real} class:potential={a.tier === TIER.potential} onclick={() => openHistory(a)}>
      <span class="rank">{a.num || '—'}</span>
      <span class="name">
        {a.name}
        {#if a.quotas?.length}<span class="tag">{a.quotas.join(' ')}</span>{/if}
      </span>
      <span class="score">{a.score?.toFixed(1) ?? '—'}</span>
      <span class="meta">
        П{a.priority || '?'}
        {#if a.documents}· 📄{/if}
        {#if tierMark(a.tier)}· {tierMark(a.tier)}{/if}
      </span>
    </button>
  {/each}
  {#if view.length === 0}
    <p class="empty">Порожньо.</p>
  {/if}
</div>

{#if maxPage > 0}
  <div class="pager">
    <button onclick={() => (page = Math.max(0, page - 1))} disabled={page === 0}>←</button>
    <span>{page + 1} / {maxPage + 1}</span>
    <button onclick={() => (page = Math.min(maxPage, page + 1))} disabled={page === maxPage}>→</button>
  </div>
{/if}

{#if openRow}
  <div class="backdrop" onclick={closeHistory} role="presentation">
    <div class="modal" onclick={(e) => e.stopPropagation()} role="dialog" aria-modal="true" aria-labelledby="appmodal-title">
      <header>
        <strong id="appmodal-title">{openRow.name}</strong>
        <button class="x" onclick={closeHistory}>✕</button>
      </header>
      <div class="detail">
        <div>Бал: <b>{openRow.score?.toFixed(2)}</b> · пріоритет {openRow.priority || '?'}</div>
        <div class="links">
          {#if openRow.calc_link}<a href={openRow.calc_link} target="_blank" rel="noreferrer">калькулятор</a>{/if}
          {#if openRow.abit_link}<a href={openRow.abit_link} target="_blank" rel="noreferrer">abit-poisk</a>{/if}
        </div>
      </div>
      <h4>Інші заяви</h4>
      {#if histLoading}
        <p class="muted">Шукаю…</p>
      {:else if histErr}
        <p class="muted">{histErr}</p>
      {:else if history && history.length}
        {#if histConfident}
          <p class="note ok">✓ Звірено за балом документа про освіту — точно ця людина</p>
        {:else}
          <p class="note warn">⚠️ abit-poisk шукає за прізвищем та ініціалами — у списку можуть бути однофамільці</p>
        {/if}
        <ul class="hist">
          {#each history as e}
            <li>
              <b>{e.university}</b> — {e.specialty}
              <span class="muted">П{e.priority} · {e.status}{e.total_score ? ` · ${e.total_score}` : ''}</span>
            </li>
          {/each}
        </ul>
      {:else}
        <p class="muted">Інших заяв не знайдено.</p>
      {/if}
    </div>
  </div>
{/if}

<style>
  .bar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin: 0.5rem 0;
  }
  .toggle {
    display: flex;
    gap: 0.4rem;
    align-items: center;
    font-size: 0.9rem;
    cursor: pointer;
  }
  .count {
    color: var(--muted);
    font-size: 0.85rem;
  }
  .rows {
    display: flex;
    flex-direction: column;
    border: 1px solid var(--border);
    border-radius: 12px;
    overflow: hidden;
  }
  .row {
    display: grid;
    grid-template-columns: 2.5rem 1fr auto auto;
    gap: 0.6rem;
    align-items: center;
    padding: 0.55rem 0.8rem;
    border: none;
    border-bottom: 1px solid var(--border);
    background: var(--card);
    text-align: left;
    cursor: pointer;
    font: inherit;
    color: inherit;
  }
  .row:last-child { border-bottom: none; }
  .row:hover { background: var(--hover); }
  .row.competitor { background: color-mix(in srgb, #dc2626 5%, var(--card)); }
  .row.potential { background: color-mix(in srgb, #f59e0b 6%, var(--card)); }
  .legend { font-size: 0.72rem; color: var(--muted); margin: 0 0 0.6rem; line-height: 1.4; }
  .rank { color: var(--muted); font-variant-numeric: tabular-nums; }
  .name { display: flex; gap: 0.4rem; align-items: center; min-width: 0; }
  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .score { font-weight: 700; font-variant-numeric: tabular-nums; }
  .meta { color: var(--muted); font-size: 0.82rem; white-space: nowrap; }
  .tag {
    font-size: 0.7rem;
    background: var(--accent-soft);
    color: var(--accent);
    padding: 0.05em 0.4em;
    border-radius: 6px;
  }
  .pager {
    display: flex;
    gap: 1rem;
    justify-content: center;
    align-items: center;
    margin-top: 0.8rem;
  }
  .empty { padding: 1rem; text-align: center; color: var(--muted); }
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.4);
    display: grid;
    place-items: center;
    padding: 1rem;
    z-index: 50;
  }
  .modal {
    background: var(--card);
    border-radius: 16px;
    padding: 1.2rem;
    max-width: 520px;
    width: 100%;
    max-height: 80vh;
    overflow: auto;
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
  }
  .modal header { display: flex; justify-content: space-between; align-items: center; }
  .x { border: none; background: none; font-size: 1.1rem; cursor: pointer; color: var(--muted); }
  .detail { margin: 0.5rem 0; }
  .links { display: flex; gap: 0.8rem; margin-top: 0.3rem; }
  .hist { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 0.5rem; }
  .hist li { padding: 0.5rem 0.7rem; background: var(--hover); border-radius: 8px; font-size: 0.9rem; }
  .muted { color: var(--muted); }
  .note { font-size: 0.8rem; margin: 0 0 0.6rem; padding: 0.4rem 0.6rem; border-radius: 8px; }
  .note.ok { color: #15803d; background: rgba(34, 197, 94, 0.12); }
  .note.warn { color: #b45309; background: rgba(245, 158, 11, 0.12); }
</style>
