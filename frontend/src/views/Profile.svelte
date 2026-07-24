<script>
  import { profileFilled } from '../lib/state.svelte.js'
  import ProfileForm from '../lib/ProfileForm.svelte'
</script>

<section>
  <h1>Профіль</h1>
  <p class="lead">Бали зберігаються лише у тебе — нікуди не надсилаються.</p>
  <p class="status" class:ok={profileFilled()}>
    {profileFilled()
      ? '✅ Профіль готовий'
      : '⚠️ Впиши 3 обов’язкові предмети + 1 предмет на вибір'}
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
