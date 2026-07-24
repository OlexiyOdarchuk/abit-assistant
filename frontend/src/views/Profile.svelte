<script>
  import { profileFilled } from '../lib/state.svelte.js'
  import ProfileForm from '../lib/ProfileForm.svelte'
</script>

<section>
  <h1>Профіль</h1>
  <p class="lead">Бали зберігаються лише у твоєму браузері — нічого не йде на сервер.</p>
  <p class="status" class:ok={profileFilled()}>
    {profileFilled() ? '✅ Профіль готовий' : '⚠️ Введи всі три обов’язкові предмети НМТ'}
  </p>
  <div class="card">
    <ProfileForm />
  </div>
  <a class="back" href="#/home">← На головну</a>
</section>

<style>
  .lead { color: var(--muted); margin-top: -0.5rem; }
  .status { font-weight: 700; }
  .status.ok { color: var(--safety); }
  .back { display: inline-block; margin-top: 0.5rem; }
</style>
