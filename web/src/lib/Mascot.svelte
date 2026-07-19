<script>
  // Абік — the admission-helper mascot: a friendly rounded character in a
  // graduation cap (on-theme for вступ/НМТ, and NOT a rhombus). Cheap inline
  // SVG. `look` switches the expression (happy | think).
  let { size = 72, look = 'happy' } = $props()
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 100 100"
  class="mascot"
  role="img"
  aria-label="Абік"
>
  <!-- soft shadow -->
  <ellipse cx="50" cy="93" rx="24" ry="5" fill="rgba(0,0,0,.12)" />

  <!-- body: rounded square (NOT a rhombus) -->
  <rect x="26" y="40" width="48" height="46" rx="15" fill="var(--accent)" />

  <!-- face -->
  <g>
    <circle cx="41" cy="58" r="6" fill="#fff" />
    <circle cx="59" cy="58" r="6" fill="#fff" />
    <circle cx={look === 'think' ? 39 : 42} cy="59" r="2.8" fill="#0b1220" />
    <circle cx={look === 'think' ? 57 : 60} cy="59" r="2.8" fill="#0b1220" />
    {#if look === 'think'}
      <line x1="44" y1="72" x2="58" y2="72" stroke="#0b1220" stroke-width="2.4" stroke-linecap="round" />
    {:else}
      <path d="M42 70 Q50 78 58 70" fill="none" stroke="#0b1220" stroke-width="2.6" stroke-linecap="round" />
    {/if}
    <!-- cheeks -->
    <circle cx="33" cy="66" r="2.6" fill="#fff" opacity=".35" />
    <circle cx="67" cy="66" r="2.6" fill="#fff" opacity=".35" />
  </g>

  <!-- graduation cap -->
  <g>
    <!-- mortarboard (flat diamond = the board seen in perspective) -->
    <polygon points="50,13 85,26 50,39 15,26" fill="#172554" />
    <!-- centre button -->
    <circle cx="50" cy="26" r="3" fill="#f6c453" />
    <!-- tassel -->
    <path d="M50 26 L82 26 L82 41" fill="none" stroke="#f6c453" stroke-width="2.2" stroke-linecap="round" />
    <circle cx="82" cy="44" r="3.2" fill="#f6c453" />
  </g>
</svg>

<style>
  .mascot { display: block; }
</style>
