<script>
  // Distribution of applicant scores with the user's position marked — the
  // "where do I stand" payoff. Pure inline SVG, themed, responsive.
  let { scores = [], userScore = 0 } = $props()

  const BINS = 26
  const W = 600
  const H = 170
  const PAD_B = 22 // room for axis labels

  let data = $derived(scores.filter((s) => Number.isFinite(s) && s > 0))

  let model = $derived.by(() => {
    if (data.length < 4) return null
    let lo = Math.floor(Math.min(...data, userScore || Infinity))
    let hi = Math.ceil(Math.max(...data, userScore || -Infinity))
    if (hi - lo < 1) hi = lo + 1
    const span = hi - lo
    const bw = span / BINS
    const bins = new Array(BINS).fill(0)
    for (const s of data) {
      let i = Math.floor((s - lo) / bw)
      if (i >= BINS) i = BINS - 1
      if (i < 0) i = 0
      bins[i]++
    }
    const maxCount = Math.max(...bins, 1)
    const userI = userScore ? Math.min(BINS - 1, Math.max(0, Math.floor((userScore - lo) / bw))) : -1
    const ux = userScore ? ((userScore - lo) / span) * W : -1
    return { lo, hi, span, bins, maxCount, userI, ux }
  })

  let pct = $derived.by(() => {
    if (!data.length || !userScore) return null
    const worse = data.filter((s) => s < userScore).length
    return Math.round((worse / data.length) * 100)
  })

  const barH = H - PAD_B
  const slot = W / BINS
</script>

{#if model}
  <figure class="hist">
    {#if pct !== null}
      <figcaption>Ти випереджаєш <b>{pct}%</b> із {data.length} заяв за балом</figcaption>
    {/if}
    <svg viewBox="0 0 {W} {H}" preserveAspectRatio="none" role="img" aria-label="Розподіл балів">
      {#each model.bins as c, i}
        {@const h = (c / model.maxCount) * (barH - 6)}
        <rect
          x={i * slot + 1}
          y={barH - h}
          width={slot - 2}
          height={h}
          rx="1.5"
          class="bar"
          class:user={i === model.userI}
        />
      {/each}
      {#if model.ux >= 0}
        <line x1={model.ux} x2={model.ux} y1="0" y2={barH} class="uline" />
        <polygon points="{model.ux - 5},0 {model.ux + 5},0 {model.ux},7" class="uflag" />
      {/if}
      <line x1="0" x2={W} y1={barH} y2={barH} class="axis" />
    </svg>
    <div class="scale">
      <span class="mono">{model.lo}</span>
      {#if model.ux >= 0}<span class="mono you" style="left: {(model.ux / W) * 100}%">▲ ти {userScore.toFixed(0)}</span>{/if}
      <span class="mono">{model.hi}</span>
    </div>
  </figure>
{/if}

<style>
  .hist { margin: 0.4rem 0 0; }
  figcaption { color: var(--muted); font-size: 0.88rem; margin-bottom: 0.5rem; }
  figcaption b { color: var(--accent); }
  svg { width: 100%; height: 150px; display: block; }
  .bar { fill: color-mix(in srgb, var(--accent) 28%, transparent); transition: fill 0.2s; }
  .bar.user { fill: var(--accent); }
  .uline { stroke: var(--reach); stroke-width: 2; stroke-dasharray: 4 3; }
  .uflag { fill: var(--reach); }
  .axis { stroke: var(--border); stroke-width: 1; }
  .scale {
    position: relative;
    display: flex;
    justify-content: space-between;
    color: var(--muted);
    font-size: 0.75rem;
    margin-top: 0.2rem;
  }
  .scale .you {
    position: absolute;
    transform: translateX(-50%);
    color: var(--reach);
    font-weight: 700;
    white-space: nowrap;
  }
</style>
