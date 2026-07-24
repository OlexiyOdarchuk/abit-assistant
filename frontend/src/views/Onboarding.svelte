<script>
  import { profileFilled, completeOnboarding } from '../lib/state.svelte.js'
  import ProfileForm from '../lib/ProfileForm.svelte'
  import Mascot from '../lib/Mascot.svelte'

  function finish() {
    if (!profileFilled()) return
    completeOnboarding()
    location.hash = '#/home'
  }
</script>

<div class="onb grid-bg">
  <div class="onb-card rise">
    <div class="head">
      <Mascot size={64} />
      <span class="badge">Крок 1 з 1 · обов'язково</span>
    </div>
    <h1>Привіт! Я <span class="gradient-text">Абік</span>. Почнемо з балів НМТ</h1>
    <p class="lead">
      Щоб порахувати твої шанси на бюджет і знайти реальних конкурентів, мені потрібні
      бали НМТ. Вони лишаються лише у твоєму браузері — нічого не зберігаємо.
    </p>

    <div class="form">
      <ProfileForm showExtras={false} />
    </div>

    <button class="primary big" onclick={finish} disabled={!profileFilled()}>
      {profileFilled() ? 'Готово, поїхали →' : 'Впиши 3 обов’язкові + 1 на вибір'}
    </button>
  </div>
</div>

<style>
  .onb { min-height: 100vh; display: grid; place-items: start center; padding: clamp(1.5rem, 6vw, 4rem) 1.2rem; }
  .onb-card {
    width: 100%;
    max-width: 560px;
    background: var(--card);
    border: 1px solid var(--border);
    border-radius: 22px;
    box-shadow: var(--shadow-lift);
    padding: clamp(1.4rem, 5vw, 2.4rem);
  }
  .head { display: flex; align-items: center; gap: 0.8rem; margin-bottom: 0.4rem; }
  .badge {
    display: inline-block;
    border: 1px solid color-mix(in srgb, var(--accent) 35%, transparent);
    background: var(--accent-soft); color: var(--accent-ink);
    padding: 0.25rem 0.7rem; border-radius: 999px;
    font-size: 0.68rem; font-weight: 700; text-transform: uppercase; letter-spacing: 0.06em;
    vertical-align: middle;
  }
  .onb h1 { margin: 1rem 0 0; font-size: clamp(1.7rem, 6vw, 2.4rem); }
  .lead { color: var(--muted); margin: 0.8rem 0 1.4rem; }
  .form { margin-bottom: 1.4rem; }
  .primary.big { width: 100%; padding: 0.9rem; font-size: 1.05rem; }
</style>
