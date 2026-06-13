<script>
  import { getFilters, discover } from '../lib/api.js'
  import { profile, profileFilled } from '../lib/state.svelte.js'
  import { tierColor } from '../lib/chance.js'
  import Chance from '../lib/Chance.svelte'

  let filters = $state(null)
  let filtersErr = $state('')
  let galuz = $state(0)
  let regions = $state([]) // selected region codes
  let budgetOnly = $state(true)

  let loading = $state(false)
  let error = $state('')
  let result = $state(null) // {found, matches}

  $effect(() => {
    getFilters()
      .then((f) => {
        filters = f
        if (!galuz && f.industries?.length) galuz = f.industries[0].code
      })
      .catch((e) => (filtersErr = e.message))
  })

  function toggleRegion(code) {
    const i = regions.indexOf(code)
    if (i >= 0) regions.splice(i, 1)
    else regions.push(code)
  }

  async function run() {
    if (!galuz) return
    loading = true
    error = ''
    result = null
    try {
      result = await discover({
        galuz,
        regions: [...regions],
        budgetOnly,
        limit: 30,
        profile: {
          nmt: { ...profile.nmt },
          quotas: [...profile.quotas],
          regionCoef: profile.regionCoef,
          creative: profile.creative,
        },
      })
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  const goAnalyze = (url) => (location.hash = '#/analyze/' + encodeURIComponent(url))

  let counts = $derived.by(() => {
    const c = { 3: 0, 2: 0, 1: 0 }
    for (const m of result?.matches ?? []) c[m.chanceTier] = (c[m.chanceTier] ?? 0) + 1
    return c
  })
</script>

<section>
  <h1>Куди я вступлю</h1>
  <p class="lead">Обери галузь і області — знайду бюджетні бакалаврські програми й покажу, куди ти проходиш.</p>

  {#if !profileFilled()}
    <p class="hint">⚠️ Спочатку заповни <a href="#/profile">профіль</a> — без балів НМТ шанси не порахувати.</p>
  {/if}
  {#if filtersErr}<p class="error">⚠️ {filtersErr}</p>{/if}

  {#if filters}
    <div class="controls card">
      <label class="field">
        <span>Галузь</span>
        <select bind:value={galuz}>
          {#each filters.industries as ind}
            <option value={ind.code}>{ind.letter ? ind.letter + ' — ' : ''}{ind.name}</option>
          {/each}
        </select>
      </label>

      <div class="field">
        <span>Області <small>(нічого не обрано = вся Україна)</small></span>
        <div class="regions">
          {#each filters.regions as r}
            <button
              class="chip"
              class:on={regions.includes(r.code)}
              onclick={() => toggleRegion(r.code)}
              type="button"
            >
              {r.name}
            </button>
          {/each}
        </div>
      </div>

      <label class="toggle">
        <input type="checkbox" bind:checked={budgetOnly} />
        Лише бюджет
      </label>

      <button class="primary" onclick={run} disabled={loading}>
        {loading ? 'Шукаю…' : '🔎 Шукати'}
      </button>
    </div>
  {:else if !filtersErr}
    <p class="muted">Завантажую фільтри…</p>
  {/if}

  {#if error}<p class="error">⚠️ {error}</p>{/if}

  {#if result}
    {#if result.matches.length === 0}
      <p class="muted">За цим фільтром нічого не знайшов — спробуй іншу галузь чи область.</p>
    {:else}
      <p class="summary">
        🟢 надійних: <b>{counts[3]}</b> · 🟡 на межі: <b>{counts[2]}</b> · 🔴 амбіційних: <b>{counts[1]}</b>
        {#if result.found > result.matches.length}<span class="muted"> · знайдено {result.found}, показую найкращі {result.matches.length}</span>{/if}
      </p>
      <div class="matches">
        {#each result.matches as m (m.url)}
          <button class="match" onclick={() => goAnalyze(m.url)}>
            <div class="m-main">
              <strong>{m.university}</strong>
              <span class="m-spec">{m.specialty || m.program}</span>
            </div>
            <div class="m-side">
              <Chance emoji={m.emoji} label={m.chance} color={tierColor(m.chanceTier)} />
              {#if m.rank > 0}<span class="m-rank">{m.rank}-й · місць {m.remaining}</span>{/if}
            </div>
          </button>
        {/each}
      </div>
    {/if}
  {/if}
</section>

<style>
  .lead { color: var(--muted); margin-top: -0.5rem; }
  .hint { color: var(--muted); font-size: 0.9rem; }
  .error { color: #dc2626; }
  .muted { color: var(--muted); }
  .controls { display: flex; flex-direction: column; gap: 1rem; }
  .field { display: flex; flex-direction: column; gap: 0.4rem; }
  .field > span { font-weight: 600; font-size: 0.9rem; }
  .field small { font-weight: 400; color: var(--muted); }
  .regions { display: flex; flex-wrap: wrap; gap: 0.4rem; }
  .chip {
    border: 1px solid var(--border);
    background: var(--card);
    border-radius: 999px;
    padding: 0.3em 0.8em;
    font-size: 0.85rem;
    cursor: pointer;
  }
  .chip.on { background: var(--accent); color: #fff; border-color: var(--accent); }
  .toggle { display: flex; gap: 0.5rem; align-items: center; cursor: pointer; }
  .summary { margin: 1rem 0 0.6rem; }
  .matches { display: flex; flex-direction: column; gap: 0.5rem; }
  .match {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 1rem;
    text-align: left;
    padding: 0.7rem 0.9rem;
    border: 1px solid var(--border);
    border-radius: 12px;
    background: var(--card);
    cursor: pointer;
    font: inherit;
    color: inherit;
  }
  .match:hover { background: var(--hover); }
  .m-main { display: flex; flex-direction: column; min-width: 0; }
  .m-main strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .m-spec { color: var(--muted); font-size: 0.85rem; }
  .m-side { display: flex; flex-direction: column; align-items: flex-end; gap: 0.3rem; white-space: nowrap; }
  .m-rank { color: var(--muted); font-size: 0.8rem; }
</style>
