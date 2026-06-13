<script>
  import { analyze, simulate } from '../lib/api.js'
  import { profile, profileFilled, saveList, removeList, isSaved } from '../lib/state.svelte.js'
  import { chanceMeta } from '../lib/chance.js'
  import Chance from '../lib/Chance.svelte'
  import Applicants from '../lib/Applicants.svelte'

  let { initialUrl = '' } = $props()

  let url = $state(initialUrl)
  let loading = $state(false)
  let error = $state('')
  let result = $state(null) // {program, userScore, analysis, applicants}

  let sim = $state(null)
  let simLoading = $state(false)
  let simError = $state('')

  async function run() {
    const u = url.trim()
    if (!u) return
    loading = true
    error = ''
    result = null
    sim = null
    try {
      result = await analyze(u, snapshot())
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  async function runSim() {
    if (!result) return
    simLoading = true
    simError = ''
    sim = null
    try {
      sim = await simulate(url.trim(), snapshot())
    } catch (e) {
      simError = e.message
    } finally {
      simLoading = false
    }
  }

  // plain snapshot of the reactive profile for the request body
  const snapshot = () => ({
    nmt: { ...profile.nmt },
    quotas: [...profile.quotas],
    regionCoef: profile.regionCoef,
    creative: profile.creative,
  })

  function toggleSave() {
    if (!result) return
    if (isSaved(result.program.url)) removeList(result.program.url)
    else
      saveList({
        url: result.program.url,
        university: result.program.university,
        program: result.program.program,
      })
  }

  // auto-run if we arrived with a URL (e.g. from Discover)
  $effect(() => {
    if (initialUrl && !result && !loading) run()
  })

  let an = $derived(result?.analysis)
  let meta = $derived(result ? chanceMeta(an.chance) : null)
</script>

<section>
  <h1>Аналіз програми</h1>
  <p class="lead">Встав посилання на програму з vstup.osvita.ua — покажу твій бал, шанси і реальних конкурентів.</p>

  <form class="search" onsubmit={(e) => { e.preventDefault(); run() }}>
    <input
      type="url"
      bind:value={url}
      placeholder="https://vstup.osvita.ua/y2025/r14/282/1471029/"
    />
    <button class="primary" disabled={loading}>{loading ? 'Аналізую…' : 'Аналізувати'}</button>
  </form>

  {#if !profileFilled()}
    <p class="hint">⚠️ Заповни <a href="#/profile">профіль</a> (бали НМТ), щоб бачити шанси — без нього лише список.</p>
  {/if}

  {#if error}<p class="error">⚠️ {error}</p>{/if}

  {#if result}
    <article class="card prog">
      <h2>{result.program.university}</h2>
      <p class="subtitle">{result.program.program}{result.program.specCode ? ` · ${result.program.specCode}` : ''}</p>

      {#if result.userScore > 0}
        <div class="headline">
          <div class="score-box">
            <span class="score-num">{result.userScore.toFixed(2)}</span>
            <span class="score-cap">твій бал</span>
          </div>
          <Chance big emoji={meta.emoji} label={meta.label} color={meta.color} />
        </div>

        <dl class="breakdown">
          <div><dt>Конкурентів</dt><dd>{an.competitors_total}</dd></div>
          {#if an.already_enrolled > 0}<div><dt>На наказі</dt><dd>{an.already_enrolled}</dd></div>{/if}
          {#if an.budget_total > 0}<div><dt>Бюджетних місць</dt><dd>{an.budget_total}</dd></div>{/if}
          {#if an.quota1_total > 0}<div><dt>Квота 1</dt><dd>{an.quota1_total}</dd></div>{/if}
          {#if an.quota2_total > 0}<div><dt>Квота 2</dt><dd>{an.quota2_total}</dd></div>{/if}
          <div><dt>Вільних місць</dt><dd>{an.remaining_spots}</dd></div>
          {#if an.my_real_rank > 0}<div><dt>Твоє місце</dt><dd>{an.my_real_rank}</dd></div>{/if}
        </dl>
        {#if an.advice}<p class="advice">💡 {an.advice}</p>{/if}
      {:else}
        <p class="hint">Заповни профіль, щоб порахувати шанси. Нижче — повний список заяв.</p>
      {/if}

      <div class="actions">
        <button onclick={toggleSave}>{isSaved(result.program.url) ? '💾 Збережено' : '💾 Зберегти'}</button>
        {#if result.userScore > 0}
          <button onclick={runSim} disabled={simLoading}>🔮 {simLoading ? 'Уточнюю…' : 'Хто піде деінде'}</button>
        {/if}
        <a class="btn-link" href={result.program.url} target="_blank" rel="noreferrer">osvita ↗</a>
      </div>

      {#if simError}<p class="error">⚠️ {simError}</p>{/if}
      {#if sim}
        <div class="sim">
          {#if sim.departures.length === 0}
            <p class="muted">Поки нікого не зняти: ніхто з тих, хто вище, ще не проходить на вищий пріоритет деінде. Працює сильніше під час хвиль рекомендацій.</p>
          {:else}
            {@const before = chanceMeta(sim.baseline.chance)}
            {@const after = chanceMeta(sim.refined.chance)}
            <p class="sim-head">
              <Chance emoji={before.emoji} label={before.label} color={before.color} /> →
              <Chance emoji={after.emoji} label={after.label} color={after.color} />
              {#if sim.baseline.my_real_rank && sim.refined.my_real_rank}
                · місце {sim.baseline.my_real_rank} → <b>{sim.refined.my_real_rank}</b>
              {/if}
            </p>
            <p class="muted">Підуть на вищий пріоритет деінде: <b>{sim.departures.length}</b></p>
            <ul class="dep">
              {#each sim.departures.slice(0, 10) as d}
                <li>{d.predicted ? '🔮' : '✅'} {d.name} → {d.university || 'інший ЗВО'} (П{d.priority})</li>
              {/each}
            </ul>
            <p class="tiny muted">🔮 прогноз за балом · ✅ підтверджено статусом</p>
          {/if}
        </div>
      {/if}
    </article>

    <h3>Список заяв</h3>
    <Applicants applicants={result.applicants} />
  {/if}
</section>

<style>
  .lead { color: var(--muted); margin-top: -0.5rem; }
  .search { display: flex; gap: 0.6rem; margin: 1rem 0; }
  .search input { flex: 1; }
  .hint { color: var(--muted); font-size: 0.9rem; }
  .error { color: #dc2626; }
  .prog h2 { margin: 0 0 0.2rem; }
  .subtitle { color: var(--muted); margin: 0 0 1rem; }
  .headline { display: flex; gap: 1rem; align-items: center; margin-bottom: 1rem; flex-wrap: wrap; }
  .score-box { display: flex; flex-direction: column; line-height: 1; }
  .score-num { font-size: 2rem; font-weight: 800; color: var(--accent); }
  .score-cap { font-size: 0.75rem; color: var(--muted); }
  .breakdown { display: grid; grid-template-columns: repeat(auto-fit, minmax(130px, 1fr)); gap: 0.6rem; margin: 0; }
  .breakdown div { background: var(--hover); border-radius: 10px; padding: 0.5rem 0.7rem; }
  .breakdown dt { font-size: 0.75rem; color: var(--muted); }
  .breakdown dd { margin: 0.1rem 0 0; font-size: 1.3rem; font-weight: 700; }
  .advice { margin-top: 0.8rem; }
  .actions { display: flex; gap: 0.6rem; margin-top: 1rem; flex-wrap: wrap; align-items: center; }
  .btn-link { align-self: center; font-size: 0.9rem; }
  .sim { margin-top: 1rem; padding: 0.9rem; background: var(--hover); border-radius: 12px; }
  .sim-head { display: flex; gap: 0.4rem; align-items: center; flex-wrap: wrap; }
  .dep { margin: 0.5rem 0 0; padding-left: 1.1rem; font-size: 0.9rem; }
  .tiny { font-size: 0.78rem; }
  .muted { color: var(--muted); }
</style>
