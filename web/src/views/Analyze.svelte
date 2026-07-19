<script>
  import { analyze, simulate } from '../lib/api.js'
  import { profile, profileFilled, saveList, removeList, isSaved, addHistory, history } from '../lib/state.svelte.js'
  import { chanceMeta } from '../lib/chance.js'
  import Chance from '../lib/Chance.svelte'
  import Applicants from '../lib/Applicants.svelte'
  import Loading from '../lib/Loading.svelte'
  import Histogram from '../lib/Histogram.svelte'
  import ChanceLegend from '../lib/ChanceLegend.svelte'

  const analyzePhrases = [
    'Відкриваю сторінку програми…',
    'Тягну список заяв з osvita…',
    'Рахую реальних конкурентів…',
    'Визначаю твоє місце і шанс…',
    'Майже готово…',
  ]
  const simPhrases = [
    'Дивлюсь, хто куди ще подався…',
    'Перевіряю abit-poisk…',
    'Хто проходить на вищий пріоритет деінде…',
    'Перераховую твої шанси…',
  ]

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
      addHistory({
        url: result.program.url,
        university: result.program.university,
        program: result.program.program,
      })
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
    creative: profile.creative,
  })

  const open = (u) => (location.hash = '#/analyze/' + encodeURIComponent(u))

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

  // Human text for the non-fatal warning codes Analyze may attach.
  const WARNINGS = {
    'budget-volume-is-ceiling':
      'Кількість місць — це максимальний обсяг держзамовлення (стеля). Реальних бюджетних місць може бути менше, тож шанс — оптимістична оцінка.',
    'license-volume-missing':
      'Ліцензований обсяг не вдалося визначити — оцінка лише за рангом, без розрахунку вільних місць.',
    'field-undersubscribed':
      'Заяв поки менше, ніж бюджетних місць — тож майже всі проходять. Якщо кампанія щойно почалась, більшість заяв подадуть в останні дні й прохідний бал ще зросте.',
  }
  const warningText = (w) => WARNINGS[w] ?? w
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

  {#if loading}<Loading phrases={analyzePhrases} />{/if}

  {#if !result && !loading && history.length}
    <div class="recent">
      <h3>Нещодавні</h3>
      <div class="recent-rows">
        {#each history.slice(0, 6) as h (h.url)}
          <button class="recent-row" onclick={() => open(h.url)}>
            <strong>{h.university}</strong>
            <span class="muted">{h.program}</span>
          </button>
        {/each}
      </div>
    </div>
  {/if}

  {#if result}
    <article class="card prog">
      <h2>{result.program.university}</h2>
      <p class="subtitle">{result.program.program}{result.program.specCode ? ` · ${result.program.specCode}` : ''}</p>

      {#if result.userScore > 0}
        <div class="headline">
          <div class="score-box">
            <span class="score-num mono">{result.userScore.toFixed(2)}<small>/200</small></span>
            <span class="score-cap">твій конкурсний бал</span>
          </div>
          <Chance big emoji={meta.emoji} label={meta.label} color={meta.color} />
        </div>

        <dl class="breakdown">
          <div><dt>Конкурентів</dt><dd>{an.competitors_total}</dd></div>
          {#if an.already_enrolled > 0}<div><dt>На наказі</dt><dd>{an.already_enrolled}</dd></div>{/if}
          {#if an.budget_total > 0}<div><dt>Бюджетних місць</dt><dd>{an.budget_total}</dd></div>{/if}
          {#if an.quota1_total > 0}<div><dt>Квота 1</dt><dd>{an.quota1_total}</dd></div>{/if}
          {#if an.quota2_total > 0}<div><dt>Квота 2</dt><dd>{an.quota2_total}</dd></div>{/if}
          {#if an.cutoff > 0}
            <div class="ground"><dt>🎯 Прохідний бал</dt><dd>{an.cutoff.toFixed(2)}</dd></div>
            {#if an.seats_filled > 0}<div><dt>Зараховано на бюджет</dt><dd>{an.seats_filled}</dd></div>{/if}
          {:else}
            <div><dt>Вільних місць</dt><dd>{an.remaining_spots}</dd></div>
          {/if}
          {#if an.my_real_rank > 0}<div><dt>Твоє місце</dt><dd>{an.my_real_rank}</dd></div>{/if}
        </dl>
        {#if an.advice}<p class="advice">💡 {an.advice}</p>{/if}
        {#if an.warnings?.length}
          {#each an.warnings as w}
            <p class="warn">⚠️ {warningText(w)}</p>
          {/each}
        {/if}

        <Histogram scores={result.applicants.map((a) => a.score)} userScore={result.userScore} />
        <ChanceLegend />
      {:else}
        <p class="hint">Заповни профіль, щоб порахувати шанси. Нижче — повний список заяв.</p>
      {/if}

      <div class="actions">
        <button onclick={toggleSave}>{isSaved(result.program.url) ? '💾 Збережено' : '💾 Зберегти'}</button>
        {#if result.userScore > 0 && !(an.cutoff > 0)}
          <button onclick={runSim} disabled={simLoading}>🔮 {simLoading ? 'Уточнюю…' : 'Хто піде деінде'}</button>
        {/if}
        <a class="btn-link" href={result.program.url} target="_blank" rel="noreferrer">osvita ↗</a>
      </div>

      {#if simError}<p class="error">⚠️ {simError}</p>{/if}
      {#if simLoading}<Loading phrases={simPhrases} />{/if}
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
  .headline {
    display: flex; gap: 1.2rem; align-items: center; margin-bottom: 1.2rem; flex-wrap: wrap;
    padding-bottom: 1.2rem; border-bottom: 1px solid var(--border);
  }
  .score-box { display: flex; flex-direction: column; line-height: 1; }
  .score-num { font-size: clamp(2.6rem, 11vw, 3.4rem); font-weight: 700; color: var(--accent); letter-spacing: -0.03em; }
  .score-num small { font-size: 0.95rem; color: var(--muted); font-weight: 500; margin-left: 0.15rem; }
  .score-cap { font-size: 0.78rem; color: var(--muted); margin-top: 0.45rem; text-transform: uppercase; letter-spacing: 0.06em; }
  .breakdown { display: grid; grid-template-columns: repeat(auto-fit, minmax(120px, 1fr)); gap: 0.55rem; margin: 0; }
  .breakdown div {
    background: var(--bg); border: 1px solid var(--border); border-radius: 12px; padding: 0.6rem 0.75rem;
  }
  .breakdown dt { font-size: 0.7rem; color: var(--muted); text-transform: uppercase; letter-spacing: 0.05em; }
  .breakdown dd { margin: 0.2rem 0 0; font-size: 1.5rem; font-weight: 700; font-family: var(--font-mono); font-variant-numeric: tabular-nums; }
  .breakdown .ground { border-color: var(--accent); background: var(--accent-soft); }
  .breakdown .ground dd { color: var(--accent-ink); }
  .advice { margin-top: 0.9rem; padding: 0.7rem 0.9rem; background: var(--accent-soft); color: var(--accent-ink); border-radius: 12px; }
  .warn { margin-top: 0.6rem; padding: 0.7rem 0.9rem; background: color-mix(in srgb, #e6a817 18%, transparent); color: var(--ink); border-radius: 12px; font-size: 0.92rem; line-height: 1.4; }
  .actions { display: flex; gap: 0.6rem; margin-top: 1rem; flex-wrap: wrap; align-items: center; }
  .btn-link { align-self: center; font-size: 0.9rem; }
  .sim { margin-top: 1rem; padding: 0.9rem; background: var(--hover); border-radius: 12px; }
  .sim-head { display: flex; gap: 0.4rem; align-items: center; flex-wrap: wrap; }
  .dep { margin: 0.5rem 0 0; padding-left: 1.1rem; font-size: 0.9rem; }
  .tiny { font-size: 0.78rem; }
  .muted { color: var(--muted); }
  .recent-rows { display: flex; flex-direction: column; gap: 0.5rem; }
  .recent-row {
    display: flex; flex-direction: column; gap: 0.1rem; text-align: left;
    border: 1px solid var(--border); border-radius: 12px; background: var(--card);
    padding: 0.6rem 0.85rem; cursor: pointer; font: inherit; color: inherit; min-width: 0;
  }
  .recent-row:hover { background: var(--hover); }
  .recent-row strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .recent-row .muted { font-size: 0.85rem; }
</style>
