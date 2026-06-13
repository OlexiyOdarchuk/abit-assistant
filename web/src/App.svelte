<script>
  import { persist } from './lib/state.svelte.js'
  import Home from './views/Home.svelte'
  import Analyze from './views/Analyze.svelte'
  import Discover from './views/Discover.svelte'
  import Profile from './views/Profile.svelte'
  import Lists from './views/Lists.svelte'

  persist() // wire localStorage saves

  function parse() {
    const h = location.hash.replace(/^#\//, '')
    const [name, ...rest] = h.split('/')
    return { name: name || 'home', arg: rest.length ? decodeURIComponent(rest.join('/')) : '' }
  }

  let route = $state(parse())
  const onHash = () => (route = parse())

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

  const nav = [
    { id: 'analyze', label: 'Аналіз' },
    { id: 'discover', label: 'Куди вступлю' },
    { id: 'profile', label: 'Профіль' },
    { id: 'lists', label: 'Збережені' },
  ]
</script>

<div class="app">
  <header class="topbar">
    <a class="brand" href="#/home"><span class="mark">◆</span> AbitAssistant</a>
    <nav>
      {#each nav as n}
        <a href="#/{n.id}" class:active={route.name === n.id}>{n.label}</a>
      {/each}
    </nav>
    <button class="theme" onclick={toggleTheme} title="Тема" aria-label="Перемкнути тему">
      {dark ? '☀' : '☾'}
    </button>
  </header>

  <main>
    {#if route.name === 'home'}
      <Home />
    {:else if route.name === 'discover'}
      <Discover />
    {:else if route.name === 'profile'}
      <Profile />
    {:else if route.name === 'lists'}
      <Lists />
    {:else}
      {#key route.arg}
        <Analyze initialUrl={route.arg} />
      {/key}
    {/if}
  </main>

  <footer>
    Дані: vstup.osvita.ua · abit-poisk.org.ua ·
    <a href="https://t.me/AbitAssistant_bot" target="_blank" rel="noreferrer">бот у Telegram</a>
  </footer>
</div>

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
  .brand {
    font-family: var(--font-display);
    font-weight: 800;
    font-size: 1.15rem;
    letter-spacing: -0.03em;
    color: var(--text);
    text-decoration: none;
  }
  .brand .mark { color: var(--accent); }
  .theme {
    border: 1.5px solid var(--border);
    background: var(--card);
    border-radius: 10px;
    width: 2.2rem;
    height: 2.2rem;
    padding: 0;
    font-size: 1rem;
    line-height: 1;
    cursor: pointer;
  }
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
</style>
