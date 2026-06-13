<script>
  // Rotating status phrases so a slow osvita/abit-poisk fetch never feels hung.
  let { phrases = ['Зачекай…'] } = $props()
  let i = $state(0)
  $effect(() => {
    const id = setInterval(() => (i = (i + 1) % phrases.length), 1900)
    return () => clearInterval(id)
  })
</script>

<div class="loading" aria-live="polite">
  <span class="spinner" aria-hidden="true"></span>
  {#key i}
    <span class="phrase">{phrases[i]}</span>
  {/key}
</div>

<style>
  .loading {
    display: flex;
    align-items: center;
    gap: 0.7rem;
    padding: 1.1rem 0.2rem;
    color: var(--muted);
  }
  .spinner {
    width: 1.1rem;
    height: 1.1rem;
    border: 2.5px solid color-mix(in srgb, var(--accent) 25%, transparent);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.7s linear infinite;
    flex: none;
  }
  .phrase {
    font-weight: 500;
    animation: fade 0.4s ease;
  }
  @keyframes spin { to { transform: rotate(360deg); } }
  @keyframes fade { from { opacity: 0; transform: translateY(2px); } to { opacity: 1; } }
</style>
