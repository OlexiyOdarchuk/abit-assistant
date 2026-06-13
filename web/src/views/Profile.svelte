<script>
  import { profile, profileFilled, SUBJECTS, REQUIRED_SUBJECTS, QUOTAS } from '../lib/state.svelte.js'

  const isRequired = (s) => REQUIRED_SUBJECTS.includes(s)

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

<section>
  <h1>Профіль</h1>
  <p class="lead">Бали зберігаються лише у твоєму браузері. Обов'язкові предмети — 🔒.</p>

  <p class="status" class:ok={profileFilled()}>
    {profileFilled() ? '✅ Профіль готовий' : '⚠️ Введи всі три обов’язкові предмети НМТ'}
  </p>

  <div class="card">
    <h3>Бали НМТ</h3>
    <div class="grid">
      {#each SUBJECTS as s}
        <label class="subj">
          <span>{isRequired(s) ? '🔒 ' : ''}{s}</span>
          <input
            type="number"
            min="100"
            max="200"
            step="0.001"
            placeholder="—"
            value={profile.nmt[s] ?? ''}
            oninput={(e) => setScore(s, e.currentTarget.value)}
          />
        </label>
      {/each}
    </div>
  </div>

  <div class="card">
    <h3>Квоти</h3>
    <div class="chips">
      {#each QUOTAS as q}
        <button
          class="chip"
          class:on={profile.quotas.includes(q.code)}
          onclick={() => toggleQuota(q.code)}
          type="button"
        >
          {q.label}
        </button>
      {/each}
    </div>
  </div>

  <div class="card row">
    <label class="toggle">
      <input type="checkbox" bind:checked={profile.regionCoef} />
      Регіональний коефіцієнт (РК)
    </label>
    <label class="subj inline">
      <span>🎨 Творчий конкурс</span>
      <input
        type="number"
        min="100"
        max="200"
        placeholder="—"
        value={profile.creative || ''}
        oninput={(e) => setCreative(e.currentTarget.value)}
      />
    </label>
  </div>
</section>

<style>
  .lead { color: var(--muted); margin-top: -0.5rem; }
  .status { font-weight: 600; }
  .status.ok { color: #16a34a; }
  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 0.7rem; }
  .subj { display: flex; flex-direction: column; gap: 0.25rem; font-size: 0.9rem; }
  .subj input { width: 100%; }
  .subj.inline { max-width: 220px; }
  .chips { display: flex; gap: 0.5rem; flex-wrap: wrap; }
  .chip {
    border: 1px solid var(--border);
    background: var(--card);
    border-radius: 999px;
    padding: 0.35em 0.9em;
    cursor: pointer;
    font: inherit;
  }
  .chip.on { background: var(--accent); color: #fff; border-color: var(--accent); }
  .row { display: flex; gap: 2rem; align-items: flex-end; flex-wrap: wrap; }
  .toggle { display: flex; gap: 0.5rem; align-items: center; cursor: pointer; }
</style>
