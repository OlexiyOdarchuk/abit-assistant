<script>
  import { profile, REQUIRED_SUBJECTS, ELECTIVE_SUBJECTS, QUOTAS } from './state.svelte.js'

  // showExtras controls only the truly-optional block (quotas + creative).
  let { showExtras = true } = $props()

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

  let hasElective = $derived(ELECTIVE_SUBJECTS.some((s) => Number(profile.nmt[s]) > 0))
</script>

<div class="pf">
  <div class="block">
    <p class="block-label">Обов'язкові предмети НМТ</p>
    <div class="grid">
      {#each REQUIRED_SUBJECTS as s}
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

  <div class="block">
    <p class="block-label">
      4-й предмет на вибір
      <span class="req-mark" class:done={hasElective}>{hasElective ? '✓' : 'обов’язково'}</span>
    </p>
    <p class="hint">НМТ — це 4 предмети. Впиши бал хоча б за один із цих (свій 4-й):</p>
    <div class="grid">
      {#each ELECTIVE_SUBJECTS as s}
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

  <details class="extras" open={showExtras}>
    <summary>Квоти та творчий конкурс (необов'язково)</summary>
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
    <div class="block">
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
  .pf { display: flex; flex-direction: column; gap: 1rem; }
  .block-label {
    font-size: 0.78rem; font-weight: 700; letter-spacing: 0.06em; text-transform: uppercase;
    color: var(--muted); margin: 0 0 0.5rem; display: flex; align-items: center; gap: 0.5rem;
  }
  .req-mark {
    font-size: 0.62rem; letter-spacing: 0.04em; padding: 0.1rem 0.45rem; border-radius: 999px;
    background: var(--reach-soft); color: var(--reach);
  }
  .req-mark.done { background: var(--safety-soft); color: var(--safety); }
  .hint { color: var(--muted); font-size: 0.85rem; margin: -0.3rem 0 0.6rem; }
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
</style>
