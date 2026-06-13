<script>
  import { persist } from './lib/state.svelte.js'
  import Analyze from './views/Analyze.svelte'
  import Discover from './views/Discover.svelte'
  import Profile from './views/Profile.svelte'
  import Lists from './views/Lists.svelte'

  persist() // wire localStorage saves

  function parse() {
    const h = location.hash.replace(/^#\//, '')
    const [name, ...rest] = h.split('/')
    return { name: name || 'analyze', arg: rest.length ? decodeURIComponent(rest.join('/')) : '' }
  }

  let route = $state(parse())
  const onHash = () => (route = parse())

  $effect(() => {
    window.addEventListener('hashchange', onHash)
    return () => window.removeEventListener('hashchange', onHash)
  })

  const nav = [
    { id: 'analyze', label: 'Аналіз' },
    { id: 'discover', label: 'Куди вступлю' },
    { id: 'profile', label: 'Профіль' },
    { id: 'lists', label: 'Збережені' },
  ]
</script>

<div class="app">
  <header class="topbar">
    <a class="brand" href="#/analyze">🎓 AbitAssistant</a>
    <nav>
      {#each nav as n}
        <a href="#/{n.id}" class:active={route.name === n.id}>{n.label}</a>
      {/each}
    </nav>
  </header>

  <main>
    {#if route.name === 'discover'}
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
  .brand { font-weight: 800; font-size: 1.1rem; color: var(--text); text-decoration: none; }
  nav { display: flex; gap: 0.3rem; flex-wrap: wrap; }
  nav a {
    padding: 0.4em 0.8em;
    border-radius: 8px;
    color: var(--muted);
    text-decoration: none;
    font-weight: 600;
    font-size: 0.92rem;
  }
  nav a:hover { background: var(--hover); color: var(--text); }
  nav a.active { background: var(--accent-soft); color: var(--accent); }
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
