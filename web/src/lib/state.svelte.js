// Client-side state (Svelte 5 runes), persisted to localStorage. v1 has no
// accounts — the profile and saved lists live in the browser.

// Subjects the profile lets you enter (must match osvita's subject names so
// ComputeRating picks them up). First three are the required НМТ subjects.
export const REQUIRED_SUBJECTS = ['Українська мова', 'Математика', 'Історія України']
export const SUBJECTS = [
  ...REQUIRED_SUBJECTS,
  'Англійська мова',
  'Українська література',
  'Біологія',
  'Фізика',
  'Хімія',
  'Географія',
  'Інша іноземна',
]
export const QUOTAS = [
  { code: 'КВ1', label: 'Квота 1' },
  { code: 'КВ2', label: 'Квота 2' },
  { code: 'КВ3', label: 'Квота 3' },
  { code: 'СБ', label: 'Співбесіда' },
]

const PROFILE_KEY = 'aa.profile.v1'
const LISTS_KEY = 'aa.lists.v1'

function readJSON(key, fallback) {
  try {
    const v = JSON.parse(localStorage.getItem(key))
    return v ?? fallback
  } catch (_) {
    return fallback
  }
}

const savedProfile = readJSON(PROFILE_KEY, {})
export const profile = $state({
  nmt: savedProfile.nmt ?? {},
  quotas: savedProfile.quotas ?? [],
  regionCoef: savedProfile.regionCoef ?? false,
  creative: savedProfile.creative ?? 0,
})

export const lists = $state(readJSON(LISTS_KEY, []))

const ONBOARDED_KEY = 'aa.onboarded.v1'
// ui.onboarded gates the whole app: until the user finishes the profile
// step once, nothing else is reachable.
export const ui = $state({ onboarded: localStorage.getItem(ONBOARDED_KEY) === '1' })

export function completeOnboarding() {
  ui.onboarded = true
}

// persist wires reactive saves; call once from the root component.
export function persist() {
  $effect(() => {
    localStorage.setItem(PROFILE_KEY, JSON.stringify(profile))
  })
  $effect(() => {
    localStorage.setItem(LISTS_KEY, JSON.stringify(lists))
  })
  $effect(() => {
    localStorage.setItem(ONBOARDED_KEY, ui.onboarded ? '1' : '0')
  })
}

export function profileFilled() {
  return REQUIRED_SUBJECTS.every((s) => Number(profile.nmt[s]) > 0)
}

export function saveList(item) {
  if (lists.some((l) => l.url === item.url)) return
  lists.unshift({ ...item, savedAt: Date.now() })
}
export function removeList(url) {
  const i = lists.findIndex((l) => l.url === url)
  if (i >= 0) lists.splice(i, 1)
}
export function isSaved(url) {
  return lists.some((l) => l.url === url)
}
