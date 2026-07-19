<script>
  import {
    priorities, addPriority, removePriority, movePriority,
    profile, profileFilled, lists, hasPriority, MAX_PRIORITIES,
  } from '../lib/state.svelte.js'
  import { predict as fetchPredict } from '../lib/api.js'

  let urlInput = $state('')
  let addErr = $state('')
  let pred = $state(null) // {items, admittedIndex}
  let loading = $state(false)
  let predErr = $state('')
  let stale = $state(false)
  let countUnlikely = $state(true)

  const snapshot = () => ({
    nmt: { ...profile.nmt },
    quotas: [...profile.quotas],
    creative: profile.creative,
  })

  // Prediction is keyed by URL so reordering the list can't misalign rows.
  let byUrl = $derived(new Map((pred?.items ?? []).map((it) => [it.url, it])))
  let admittedUrl = $derived(
    pred && pred.admittedIndex >= 0 ? pred.items[pred.admittedIndex]?.url : null
  )

  async function runPredict() {
    if (!priorities.length || !profileFilled()) return
    loading = true
    predErr = ''
    try {
      pred = await fetchPredict(priorities.map((p) => p.url), snapshot(), !countUnlikely)
      stale = false
    } catch (e) {
      predErr = e.message
    } finally {
      loading = false
    }
  }

  function addByUrl() {
    addErr = ''
    const u = urlInput.trim()
    if (!u) return
    if (!/vstup\.osvita\.ua/.test(u)) {
      addErr = 'Це не схоже на посилання vstup.osvita.ua'
      return
    }
    if (!addPriority({ url: u })) {
      addErr = priorities.length >= MAX_PRIORITIES ? `Максимум ${MAX_PRIORITIES} пріоритетів` : 'Вже у списку'
      return
    }
    urlInput = ''
    stale = true
  }

  function addFromSaved(l) {
    if (addPriority({ url: l.url, university: l.university, program: l.program })) stale = true
  }
  function remove(i) { removePriority(i); stale = true; if (!priorities.length) pred = null }
  function move(i, d) { movePriority(i, d); stale = true }

  $effect(() => {
    // first render / toggle change → (re)compute; list edits only mark stale
    countUnlikely
    if (priorities.length && profileFilled() && !pred && !loading) runPredict()
  })

  let addable = $derived(lists.filter((l) => !hasPriority(l.url)))
</script>

<section class="wrap">
  <h1>🎯 Мій прогноз вступу</h1>
  <p class="lead">
    Склади свій список програм у порядку пріоритету (до {MAX_PRIORITIES}) — покажу, за яким пріоритетом ти
    реально вступиш. Вступ — пріоритетна модель: тебе зараховують на <b>одну</b> програму, найвищий
    пріоритет, де ти проходиш прохідний бал.
  </p>

  {#if !profileFilled()}
    <p class="warn">⚠️ Заповни <a href="#/profile">профіль</a> (бали НМТ) — без нього не порахую прохід.</p>
  {/if}

  {#if pred && !stale}
    {#if admittedUrl}
      <div class="verdict ok">
        ✅ Прогноз: проходиш за <b>пріоритетом {pred.admittedIndex + 1}</b>
        {#if pred.admittedIndex > 0}<div class="sub">вищі пріоритети поки не проходиш — вони згорять, і ти впадеш сюди</div>{/if}
      </div>
    {:else}
      <div class="verdict no">😔 За поточними даними не проходиш на жоден пріоритет. Додай запасні варіанти з нижчим прохідним.</div>
    {/if}
  {/if}

  <ol class="prio">
    {#each priorities as p, i (p.url)}
      {@const o = byUrl.get(p.url)}
      <li class:admitted={!stale && p.url === admittedUrl}>
        <span class="pos">{i + 1}</span>
        <div class="body">
          <div class="name">{o?.university || p.university || p.url}</div>
          {#if o?.program || p.program}<div class="sub">{o?.program || p.program}</div>{/if}
          {#if o && !stale}
            {#if o.fetched}
              <div class="stat">{o.emoji} {o.chance} · бал {o.score.toFixed(1)}{o.cutoff ? ` · прохідний ${o.cutoff.toFixed(1)}` : ''}</div>
            {:else}
              <div class="stat err">⚠️ не вдалося завантажити</div>
            {/if}
          {/if}
        </div>
        <div class="ctl">
          <button onclick={() => move(i, -1)} disabled={i === 0} aria-label="вгору">⬆️</button>
          <button onclick={() => move(i, +1)} disabled={i === priorities.length - 1} aria-label="вниз">⬇️</button>
          <button onclick={() => remove(i)} aria-label="прибрати">🗑</button>
        </div>
      </li>
    {/each}
    {#if !priorities.length}
      <p class="empty">Список порожній — додай першу програму.</p>
    {/if}
  </ol>

  <div class="actions">
    <button class="primary" onclick={runPredict} disabled={loading || !priorities.length || !profileFilled()} class:stale>
      {loading ? 'Рахую…' : stale ? '🔮 Оновити прогноз' : '🔮 Порахувати прогноз'}
    </button>
    <label class="toggle">
      <input type="checkbox" bind:checked={countUnlikely} onchange={runPredict} />
      Рахувати пріоритет 3+
    </label>
  </div>
  {#if predErr}<p class="warn">{predErr}</p>{/if}

  {#if priorities.length < MAX_PRIORITIES}
    <div class="add">
      <h3>➕ Додати програму</h3>
      <div class="addrow">
        <input placeholder="Посилання vstup.osvita.ua…" bind:value={urlInput} onkeydown={(e) => e.key === 'Enter' && addByUrl()} />
        <button onclick={addByUrl}>Додати</button>
      </div>
      {#if addErr}<p class="warn small">{addErr}</p>{/if}

      {#if addable.length}
        <p class="saved-cap">…або зі збережених:</p>
        <div class="saved">
          {#each addable as l (l.url)}
            <button class="chip" onclick={() => addFromSaved(l)}>+ {l.university || l.name || l.url}</button>
          {/each}
        </div>
      {/if}
    </div>
  {/if}
</section>

<style>
  .wrap { max-width: 640px; margin: 0 auto; }
  h1 { margin: 0 0 0.4rem; }
  .lead { color: var(--muted); font-size: 0.9rem; line-height: 1.5; margin: 0 0 1rem; }
  .warn { padding: 0.6rem 0.8rem; background: color-mix(in srgb, #e6a817 16%, transparent); border-radius: 10px; font-size: 0.9rem; margin: 0.6rem 0; }
  .warn.small { font-size: 0.8rem; }
  .verdict { padding: 0.8rem 1rem; border-radius: 12px; margin: 0.8rem 0; font-size: 0.95rem; }
  .verdict.ok { background: color-mix(in srgb, #22c55e 14%, var(--card)); }
  .verdict.no { background: color-mix(in srgb, #ef4444 12%, var(--card)); }
  .verdict .sub { color: var(--muted); font-size: 0.8rem; margin-top: 0.25rem; }
  ol.prio { list-style: none; padding: 0; margin: 0.5rem 0; display: flex; flex-direction: column; gap: 0.5rem; }
  ol.prio li { display: flex; align-items: center; gap: 0.6rem; padding: 0.6rem 0.7rem; background: var(--card); border: 1px solid var(--border); border-radius: 12px; }
  ol.prio li.admitted { border-color: #22c55e; background: color-mix(in srgb, #22c55e 8%, var(--card)); }
  .pos { flex: none; width: 1.6rem; height: 1.6rem; display: grid; place-items: center; border-radius: 50%; background: var(--hover); font-weight: 600; font-size: 0.85rem; }
  .body { flex: 1; min-width: 0; }
  .name { font-weight: 600; font-size: 0.92rem; }
  .body .sub { color: var(--muted); font-size: 0.8rem; }
  .stat { font-size: 0.82rem; margin-top: 0.2rem; }
  .stat.err { color: #dc2626; }
  .ctl { display: flex; gap: 0.15rem; flex: none; }
  .ctl button { background: none; border: none; cursor: pointer; font-size: 1rem; padding: 0.2rem; border-radius: 6px; }
  .ctl button:disabled { opacity: 0.3; cursor: default; }
  .empty { color: var(--muted); text-align: center; padding: 1rem; }
  .actions { display: flex; gap: 0.8rem; align-items: center; flex-wrap: wrap; margin: 0.8rem 0; }
  .primary { padding: 0.6rem 1rem; border-radius: 10px; border: none; background: var(--accent); color: #fff; font-weight: 600; cursor: pointer; }
  .primary:disabled { opacity: 0.5; cursor: default; }
  .primary.stale { box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 30%, transparent); }
  .toggle { display: flex; align-items: center; gap: 0.4rem; font-size: 0.85rem; cursor: pointer; }
  .add { margin-top: 1rem; padding-top: 1rem; border-top: 1px solid var(--border); }
  .add h3 { margin: 0 0 0.5rem; font-size: 0.95rem; }
  .addrow { display: flex; gap: 0.5rem; }
  .addrow input { flex: 1; padding: 0.55rem 0.7rem; border-radius: 10px; border: 1px solid var(--border); background: var(--card); color: var(--text); }
  .addrow button { padding: 0.55rem 1rem; border-radius: 10px; border: none; background: var(--accent); color: #fff; cursor: pointer; }
  .saved-cap { color: var(--muted); font-size: 0.82rem; margin: 0.8rem 0 0.4rem; }
  .saved { display: flex; flex-wrap: wrap; gap: 0.4rem; }
  .chip { padding: 0.35rem 0.7rem; border-radius: 999px; border: 1px solid var(--border); background: var(--card); color: var(--text); font-size: 0.82rem; cursor: pointer; }
  .chip:hover { border-color: var(--accent); }
</style>
