<script>
  // Custom title bar for the frameless desktop (Wails) window. It only renders
  // in the desktop shell — the web build has no window.runtime, so it's absent
  // and the browser's own chrome is used. Window ops go through the injected
  // window.runtime global (no import of generated bindings, so the frontend
  // still builds standalone for the web).
  const rt = typeof window !== 'undefined' ? window.runtime : null

  const minimise = () => rt?.WindowMinimise?.()
  const toggleMax = () => rt?.WindowToggleMaximise?.()
  const quit = () => rt?.Quit?.()
</script>

{#if rt}
  <div class="titlebar" ondblclick={toggleMax} role="presentation">
    <div class="drag">
      <span class="mark">🎓</span>
      <span class="name">AbitAssistant</span>
    </div>
    <div class="controls">
      <button class="ctl" onclick={minimise} title="Згорнути" aria-label="Згорнути">
        <svg viewBox="0 0 12 12" width="11" height="11"><rect x="2" y="5.4" width="8" height="1.2" fill="currentColor" /></svg>
      </button>
      <button class="ctl" onclick={toggleMax} title="Розгорнути" aria-label="Розгорнути">
        <svg viewBox="0 0 12 12" width="11" height="11"><rect x="2.5" y="2.5" width="7" height="7" fill="none" stroke="currentColor" stroke-width="1.2" /></svg>
      </button>
      <button class="ctl close" onclick={quit} title="Закрити" aria-label="Закрити">
        <svg viewBox="0 0 12 12" width="11" height="11"><path d="M3 3l6 6M9 3l-6 6" stroke="currentColor" stroke-width="1.3" fill="none" /></svg>
      </button>
    </div>
  </div>
{/if}

<style>
  .titlebar {
    position: sticky;
    top: 0;
    z-index: 60;
    height: 34px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    background: color-mix(in srgb, var(--bg) 88%, transparent);
    backdrop-filter: blur(8px);
    border-bottom: 1px solid var(--border);
    /* the whole bar is a drag handle for moving the frameless window */
    --wails-draggable: drag;
  }
  .drag {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding-left: 0.8rem;
    font-family: var(--font-display);
    font-weight: 700;
    font-size: 0.82rem;
    color: var(--muted);
    letter-spacing: -0.01em;
    user-select: none;
  }
  .drag .mark { font-size: 0.9rem; }
  .controls {
    display: flex;
    height: 100%;
    --wails-draggable: no-drag; /* buttons must not drag the window */
  }
  .ctl {
    width: 44px;
    height: 100%;
    border: 0;
    background: transparent;
    color: var(--muted);
    display: grid;
    place-items: center;
    cursor: pointer;
    transition: background 0.15s, color 0.15s;
  }
  .ctl:hover { background: var(--hover); color: var(--text); }
  .ctl.close:hover { background: #ef4444; color: #fff; }
</style>
