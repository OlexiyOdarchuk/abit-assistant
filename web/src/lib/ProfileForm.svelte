<script>
  import { profile, SUBJECTS, REQUIRED_SUBJECTS, QUOTAS } from './state.svelte.js'

  // showExtras lets the onboarding hide optional fields behind a toggle.
  let { showExtras = $bindable(true) } = $props()

  const isRequired = (s) => REQUIRED_SUBJECTS.includes(s)
  const required = SUBJECTS.filter(isRequired)
  const optional = SUBJECTS.filter((s) => !isRequired(s))

  function setScore(s, v) {
    const n = Number(v)
    if (v === '' || Number.isNaN(n) || n <= 0) delete profile.nmt[s]
    else profile.nmt[s] = Math.min(200, n)
  }
  function toggleQuota(code) {
    const i = profile.quotas.indexOf(code)
    if (i >= 0) profile.quotas.splice(i, 1)
    else profile.quotas.push(code)
  }
  function setCreative(v) {
    const n = Number(v)
    profile.creative = v === '' || Number.isNaN(n) ? 0 : Math.min(200, n)
  }
</script>

<div class="pf">
  <div class="block">
    <p class="block-label">Обов'язкові предмети НМТ</p>
    <div class="grid">
      {#each required as s}
        <label class="subj">
          <span>🔒 {s}</span>
          <input
            type="number" min="100" max="200" step="0.001" placeholder="—"
            value={profile.nmt[s] ?? ''}
            oninput={(e) => setScore(s, e.currentTarget.value)}
          />
        </label>
      {/each}
    </div>
  </div>

  <details class="extras" open={showExtras}>
    <summary>Додаткові предмети та налаштування</summary>
    <div class="block">
      <p class="block-label">4-й предмет на вибір (та інші бали)</p>
      <div class="grid">
        {#each optional as s}
          <label class="subj">
            <span>{s}</span>
            <input
              type="number" min="100" max="200" step="0.001" placeholder="—"
              value={profile.nmt[s] ?? ''}
              oninput={(e) => setScore(s, e.currentTarget.value)}
            />
          </label>
        {/each}
      </div>
    </div>

    <div class="block">
      <p class="block-label">Квоти</p>
      <div class="chips">
        {#each QUOTAS as q}
          <button class="chip" class:on={profile.quotas.includes(q.code)} onclick={() => toggleQuota(q.code)} type="button">
            {q.label}
          </button>
        {/each}
      </div>
    </div>

    <div class="block row">
      <label class="toggle">
        <input type="checkbox" bind:checked={profile.regionCoef} />
        Регіональний коефіцієнт (РК)
      </label>
      <label class="subj inline">
        <span>🎨 Творчий конкурс</span>
        <input
          type="number" min="100" max="200" placeholder="—"
          value={profile.creative || ''}
          oninput={(e) => setCreative(e.currentTarget.value)}
        />
      </label>
    </div>
  </details>
</div>

<style>
  .pf { display: flex; flex-direction: column; gap: 0.5rem; }
  .block { margin-bottom: 0.4rem; }
  .block-label { font-size: 0.78rem; font-weight: 700; letter-spacing: 0.06em; text-transform: uppercase; color: var(--muted); margin: 0 0 0.6rem; }
  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 0.7rem; }
  .subj { display: flex; flex-direction: column; gap: 0.3rem; font-size: 0.85rem; }
  .subj > span { color: var(--muted); }
  .subj input { width: 100%; font-family: var(--font-mono); font-variant-numeric: tabular-nums; }
  .subj.inline { max-width: 220px; }
  .extras { border-top: 1px solid var(--border); padding-top: 0.8rem; }
  .extras summary { cursor: pointer; font-weight: 600; color: var(--accent); margin-bottom: 0.8rem; }
  .chips { display: flex; gap: 0.5rem; flex-wrap: wrap; }
  .chip {
    border: 1.5px solid var(--border); background: var(--card);
    border-radius: 999px; padding: 0.35em 0.9em; cursor: pointer; font: inherit;
  }
  .chip.on { background: var(--accent); color: #fff; border-color: var(--accent); }
  :root.dark .chip.on { color: #07101f; }
  .row { display: flex; gap: 2rem; align-items: flex-end; flex-wrap: wrap; }
  .toggle { display: flex; gap: 0.5rem; align-items: center; cursor: pointer; }
</style>
