<script>
  // Faint classical-facade silhouette (pediment + columns) — the universal
  // "university / academia" motif, echoing official Ukrainian education style.
  // Purely decorative; sits behind content at low opacity.
  let { opacity = 0.06 } = $props()
  const cols = [32, 54, 76, 98, 120, 142, 164]
</script>

<svg class="campus" viewBox="0 0 196 86" preserveAspectRatio="xMidYMax meet" aria-hidden="true" style="--op: {opacity}">
  <polygon points="98,6 178,36 18,36" />
  <rect x="16" y="36" width="164" height="5" />
  {#each cols as x}
    <rect x={x} y="43" width="8" height="33" rx="1" />
  {/each}
  <rect x="18" y="76" width="160" height="6" />
  <rect x="10" y="82" width="176" height="4" />
</svg>

<style>
  .campus {
    width: 100%;
    height: auto;
    fill: var(--accent);
    opacity: var(--op);
    pointer-events: none;
  }
</style>
