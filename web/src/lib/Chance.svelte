<script>
  let { emoji = '❔', label = '', color = '#6b7280', big = false } = $props()
</script>

<span class="chance" class:big style="--c: {color}">{emoji} {label}</span>

<style>
  .chance {
    display: inline-flex;
    align-items: center;
    gap: 0.35em;
    padding: 0.2em 0.7em;
    border-radius: 999px;
    font-weight: 600;
    font-size: 0.85rem;
    color: var(--c);
    background: color-mix(in srgb, var(--c) 12%, transparent);
    border: 1px solid color-mix(in srgb, var(--c) 30%, transparent);
    white-space: nowrap;
  }
  .big {
    font-size: 1rem;
    padding: 0.3em 0.9em;
  }
</style>
