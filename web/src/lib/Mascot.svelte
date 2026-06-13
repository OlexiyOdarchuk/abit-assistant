<script>
  // Ромбік — the diamond mascot. On-brand (◆), cheap SVG, gives the app a face.
  let { size = 72, look = 'happy' } = $props()
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 100 100"
  class="mascot"
  role="img"
  aria-label="Ромбік"
>
  <!-- soft shadow -->
  <ellipse cx="50" cy="92" rx="22" ry="5" fill="rgba(0,0,0,.12)" />
  <!-- body: rounded diamond -->
  <rect x="22" y="22" width="56" height="56" rx="15" fill="var(--accent)" transform="rotate(45 50 50)" />
  <!-- face -->
  <g>
    <circle cx="41" cy="47" r="6.5" fill="#fff" />
    <circle cx="59" cy="47" r="6.5" fill="#fff" />
    <circle cx={look === 'think' ? 39 : 42} cy="48" r="3" fill="#0b1220" />
    <circle cx={look === 'think' ? 57 : 60} cy="48" r="3" fill="#0b1220" />
    {#if look === 'think'}
      <line x1="44" y1="62" x2="58" y2="62" stroke="#0b1220" stroke-width="2.4" stroke-linecap="round" />
    {:else}
      <path d="M42 60 Q50 68 58 60" fill="none" stroke="#0b1220" stroke-width="2.6" stroke-linecap="round" />
    {/if}
    <!-- cheeks -->
    <circle cx="34" cy="56" r="2.6" fill="#fff" opacity=".35" />
    <circle cx="66" cy="56" r="2.6" fill="#fff" opacity=".35" />
  </g>
</svg>

<style>
  .mascot { display: block; }
</style>
