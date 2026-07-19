// Client-side state (Svelte 5 runes), persisted to localStorage. v1 has no
// accounts — the profile and saved lists live in the browser.

// Subjects the profile lets you enter (must match osvita's subject names so
// ComputeRating picks them up). НМТ = 4 subjects: the three required ones
// plus exactly one elective (also mandatory — "4-й предмет на вибір").
export const REQUIRED_SUBJECTS = ['Українська мова', 'Математика', 'Історія України']
export const ELECTIVE_SUBJECTS = [
  'Англійська мова',
  'Українська література',
  'Біологія',
  'Фізика',
  'Хімія',
  'Географія',
  'Інша іноземна',
]
export const SUBJECTS = [...REQUIRED_SUBJECTS, ...ELECTIVE_SUBJECTS]
export const QUOTAS = [
  { code: 'КВ1', label: 'Квота 1' },
  { code: 'КВ2', label: 'Квота 2' },
  { code: 'КВ3', label: 'Квота 3' },
  { code: 'СБ', label: 'Співбесіда' },
]

const PROFILE_KEY = 'aa.profile.v1'
const LISTS_KEY = 'aa.lists.v1'
const HISTORY_KEY = 'aa.history.v1'
const PRIORITIES_KEY = 'aa.priorities.v1'
const HISTORY_MAX = 20
export const MAX_PRIORITIES = 5

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
  creative: savedProfile.creative ?? 0,
})

export const lists = $state(readJSON(LISTS_KEY, []))
export const history = $state(readJSON(HISTORY_KEY, []))
// priorities: the user's ranked application list (index 0 = priority 1).
// Each item: {url, university, program}. Drives the "Мій прогноз" view.
export const priorities = $state(readJSON(PRIORITIES_KEY, []))

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
    localStorage.setItem(HISTORY_KEY, JSON.stringify(history))
  })
  $effect(() => {
    localStorage.setItem(PRIORITIES_KEY, JSON.stringify(priorities))
  })
  $effect(() => {
    localStorage.setItem(ONBOARDED_KEY, ui.onboarded ? '1' : '0')
  })
}

// profileFilled: НМТ needs all 3 required subjects AND at least one elective
// (the mandatory "4-й предмет на вибір"). Without the 4th the rating is
// incomplete, so the gate must not pass.
export function profileFilled() {
  const required = REQUIRED_SUBJECTS.every((s) => Number(profile.nmt[s]) > 0)
  const elective = ELECTIVE_SUBJECTS.some((s) => Number(profile.nmt[s]) > 0)
  return required && elective
}

// addHistory records a viewed program (most-recent first, deduped, capped).
export function addHistory(item) {
  const i = history.findIndex((h) => h.url === item.url)
  if (i >= 0) history.splice(i, 1)
  history.unshift({ ...item, at: Date.now() })
  if (history.length > HISTORY_MAX) history.length = HISTORY_MAX
}
export function clearHistory() {
  history.length = 0
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

// --- priorities ------------------------------------------------------------

export function addPriority(item) {
  if (!item?.url) return false
  if (priorities.some((p) => p.url === item.url)) return false
  if (priorities.length >= MAX_PRIORITIES) return false
  priorities.push({ url: item.url, university: item.university ?? '', program: item.program ?? '' })
  return true
}
export function removePriority(i) {
  if (i >= 0 && i < priorities.length) priorities.splice(i, 1)
}
export function movePriority(i, delta) {
  const j = i + delta
  if (i < 0 || i >= priorities.length || j < 0 || j >= priorities.length) return
  const [it] = priorities.splice(i, 1)
  priorities.splice(j, 0, it)
}
export function hasPriority(url) {
  return priorities.some((p) => p.url === url)
}
