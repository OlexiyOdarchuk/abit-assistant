<script>
  import { persist, ui } from './lib/state.svelte.js'
  import WindowControls from './lib/WindowControls.svelte'
  import Onboarding from './views/Onboarding.svelte'
  import Home from './views/Home.svelte'
  import Analyze from './views/Analyze.svelte'
  import Priorities from './views/Priorities.svelte'
  import Discover from './views/Discover.svelte'
  import Profile from './views/Profile.svelte'
  import Lists from './views/Lists.svelte'
  import Help from './views/Help.svelte'

  persist() // wire localStorage saves

  function parse() {
    const h = location.hash.replace(/^#\//, '')
    const [name, ...rest] = h.split('/')
    return { name: name || 'home', arg: rest.length ? decodeURIComponent(rest.join('/')) : '' }
  }

  let route = $state(parse())
  const onHash = () => ((route = parse()), (menuOpen = false))
  $effect(() => {
    window.addEventListener('hashchange', onHash)
    return () => window.removeEventListener('hashchange', onHash)
  })

  let dark = $state(document.documentElement.classList.contains('dark'))
  function toggleTheme() {
    dark = !dark
    document.documentElement.classList.toggle('dark', dark)
    try {
      localStorage.setItem('theme', dark ? 'dark' : 'light')
    } catch (e) {}
  }

  let menuOpen = $state(false)

  function resetAll() {
    if (!confirm('Скинути профіль, збережені списки та історію? Дію не скасувати.')) return
    try {
      for (const k of Object.keys(localStorage)) if (k.startsWith('aa.')) localStorage.removeItem(k)
    } catch (e) {}
    location.hash = '#/home'
    location.reload()
  }

  const nav = [
    { id: 'home', label: 'Головна' },
    { id: 'analyze', label: 'Аналіз' },
    { id: 'priorities', label: 'Прогноз' },
    { id: 'discover', label: 'Куди вступлю' },
    { id: 'lists', label: 'Збережені' },
  ]
  const bottomNav = [
    { id: 'home', label: 'Головна', icon: '🏠' },
    { id: 'analyze', label: 'Аналіз', icon: '🔎' },
    { id: 'priorities', label: 'Прогноз', icon: '🎯' },
    { id: 'discover', label: 'Підбір', icon: '🧭' },
    { id: 'lists', label: 'Списки', icon: '💾' },
  ]
</script>

<WindowControls />

{#if !ui.onboarded}
  <!-- mandatory profile gate: nothing else until the profile is filled -->
  <header class="topbar minimal">
    <span class="brand"><span class="mark">🎓</span> AbitAssistant</span>
    <button class="icon-btn" onclick={toggleTheme} aria-label="Тема">{dark ? '☀' : '☾'}</button>
  </header>
  <Onboarding />
{:else}
  <div class="app">
    <header class="topbar">
      <a class="brand" href="#/home"><span class="mark">🎓</span> AbitAssistant</a>
      <nav>
        {#each nav as n}
          <a href="#/{n.id}" class:active={route.name === n.id}>{n.label}</a>
        {/each}
      </nav>
      <div class="actions">
        <button class="icon-btn" onclick={toggleTheme} title="Тема" aria-label="Перемкнути тему">
          {dark ? '☀' : '☾'}
        </button>
        <div class="menu-wrap">
          <button
            class="icon-btn"
            class:open={menuOpen}
            onclick={() => (menuOpen = !menuOpen)}
            title="Меню"
            aria-label="Меню"
            aria-expanded={menuOpen}>⋯</button
          >
          {#if menuOpen}
            <button class="menu-backdrop" onclick={() => (menuOpen = false)} aria-label="Закрити меню"></button>
            <div class="menu" role="menu">
              <a href="#/profile" role="menuitem"><span>👤</span> Профіль</a>
              <a href="#/help" role="menuitem"><span>❓</span> Як користуватися</a>
              <div class="sep"></div>
              <a href="https://t.me/AbitAssistant_bot" target="_blank" rel="noreferrer" role="menuitem"
                ><span>✈️</span> Бот у Telegram</a>
              <a href="https://vstup.osvita.ua" target="_blank" rel="noreferrer" role="menuitem"
                ><span>🌐</span> vstup.osvita.ua</a>
              <div class="sep"></div>
              <button class="danger" onclick={resetAll} role="menuitem"><span>🗑</span> Скинути дані</button>
            </div>
          {/if}
        </div>
      </div>
    </header>

    <main>
      {#if route.name === 'discover'}
        <Discover />
      {:else if route.name === 'priorities'}
        <Priorities />
      {:else if route.name === 'profile'}
        <Profile />
      {:else if route.name === 'help'}
        <Help />
      {:else if route.name === 'lists'}
        <Lists />
      {:else if route.name === 'analyze'}
        {#key route.arg}
          <Analyze initialUrl={route.arg} />
        {/key}
      {:else}
        <Home />
      {/if}
    </main>

    <footer>
      Дані: vstup.osvita.ua · abit-poisk.org.ua ·
      <a href="#/help">як це працює</a>
    </footer>

    <nav class="bottom">
      {#each bottomNav as n}
        <a href="#/{n.id}" class:active={route.name === n.id}>
          <span class="bi">{n.icon}</span>
          <span class="bl">{n.label}</span>
        </a>
      {/each}
    </nav>
  </div>
{/if}

<style>
  .app { display: flex; flex-direction: column; min-height: 100vh; }
  .topbar {
    position: sticky;
    top: 0;
    z-index: 10;
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 1rem;
    padding: 0.8rem clamp(1rem, 4vw, 2rem);
    background: color-mix(in srgb, var(--bg) 85%, transparent);
    backdrop-filter: blur(8px);
    border-bottom: 1px solid var(--border);
    flex-wrap: wrap;
  }
  .topbar.minimal { justify-content: space-between; }
  .brand {
    font-family: var(--font-display);
    font-weight: 800;
    font-size: 1.15rem;
    letter-spacing: -0.03em;
    color: var(--text);
    text-decoration: none;
  }
  .brand .mark { color: var(--accent); }
  .actions { display: flex; align-items: center; gap: 0.4rem; }
  .icon-btn {
    border: 1.5px solid var(--border);
    background: var(--card);
    color: var(--text);
    border-radius: 10px;
    width: 2.2rem;
    height: 2.2rem;
    padding: 0;
    font-size: 1rem;
    line-height: 1;
    cursor: pointer;
    display: grid;
    place-items: center;
    transition: border-color 0.15s, background 0.15s;
  }
  .icon-btn:hover { border-color: var(--accent); }
  .icon-btn.open { border-color: var(--accent); color: var(--accent); }

  .menu-wrap { position: relative; }
  .menu-backdrop {
    position: fixed;
    inset: 0;
    z-index: 40;
    background: transparent;
    border: 0;
    cursor: default;
  }
  .menu {
    position: absolute;
    right: 0;
    top: calc(100% + 0.5rem);
    z-index: 50;
    min-width: 220px;
    background: var(--card);
    border: 1px solid var(--border);
    border-radius: var(--r-ctrl);
    box-shadow: var(--shadow-lift);
    padding: 0.4rem;
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
    animation: pop 0.14s ease both;
  }
  @keyframes pop { from { opacity: 0; transform: translateY(-4px) scale(0.98); } to { opacity: 1; transform: none; } }
  .menu a, .menu button {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.55rem 0.7rem;
    border-radius: 9px;
    text-decoration: none;
    color: var(--text);
    font: inherit;
    font-weight: 600;
    font-size: 0.92rem;
    background: transparent;
    border: 0;
    cursor: pointer;
    text-align: left;
  }
  .menu a:hover, .menu button:hover { background: var(--hover); }
  .menu button.danger { color: var(--reach); }
  .menu .sep { height: 1px; background: var(--border); margin: 0.3rem 0.2rem; }

  nav { display: flex; gap: 0.15rem; flex-wrap: wrap; }
  nav a {
    position: relative;
    padding: 0.4em 0.7em;
    color: var(--muted);
    text-decoration: none;
    font-weight: 600;
    font-size: 0.92rem;
    border-radius: 8px;
  }
  nav a:hover { color: var(--text); }
  nav a.active { color: var(--accent); }
  nav a.active::after {
    content: '';
    position: absolute;
    left: 0.7em;
    right: 0.7em;
    bottom: -0.85rem;
    height: 2px;
    background: var(--accent);
    border-radius: 2px 2px 0 0;
  }
  main { flex: 1; width: 100%; max-width: 760px; margin: 0 auto; padding: clamp(1rem, 4vw, 2rem); }
  footer {
    text-align: center;
    padding: 1.5rem;
    color: var(--muted);
    font-size: 0.85rem;
    border-top: 1px solid var(--border);
  }
  footer a { color: var(--accent); }

  /* bottom tab bar — mobile only */
  .bottom { display: none; }
  @media (max-width: 640px) {
    .topbar nav { display: none; } /* move primary nav to the bottom bar */
    main { padding-bottom: 5.5rem; }
    footer { margin-bottom: 4rem; }
    .bottom {
      position: fixed;
      left: 0;
      right: 0;
      bottom: 0;
      z-index: 20;
      display: flex;
      justify-content: space-around;
      background: color-mix(in srgb, var(--card) 92%, transparent);
      backdrop-filter: blur(10px);
      border-top: 1px solid var(--border);
      padding: 0.4rem 0.3rem calc(0.4rem + env(safe-area-inset-bottom));
    }
    .bottom a {
      display: flex;
      flex-direction: column;
      align-items: center;
      gap: 0.15rem;
      text-decoration: none;
      color: var(--muted);
      font-size: 0.68rem;
      font-weight: 600;
      padding: 0.3rem 0.7rem;
      border-radius: 12px;
    }
    .bottom a.active { color: var(--accent); background: var(--accent-soft); }
    .bottom .bi { font-size: 1.25rem; line-height: 1; }
  }
</style>
