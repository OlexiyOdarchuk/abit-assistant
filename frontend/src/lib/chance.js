// Maps the abit.ChanceLevel int (analysis.chance) to display metadata,
// mirroring ChanceLevel.Emoji()/Label() on the Go side.
//
// Colours are THEME TOKENS (var(--…)), not literal hex, so the most important
// signal — the chance verdict — brightens correctly in dark mode instead of
// showing dull light-theme colours. Components feed them into `--c` / color-mix,
// where nested var() resolves fine.
const META = {
  0: { emoji: '❔', label: 'Невідомий', color: 'var(--muted)' },
  1: { emoji: '⚫', label: 'Нульовий', color: 'var(--muted)' },
  2: { emoji: '🔴', label: 'Низький', color: 'var(--reach)' },
  3: { emoji: '🟡', label: 'Середній', color: 'var(--match)' },
  4: { emoji: '🟢', label: 'Високий', color: 'var(--safety)' },
  5: { emoji: '🟢', label: 'Високий (Квота 1)', color: 'var(--safety)' },
  6: { emoji: '🟢', label: 'Високий (Квота 2)', color: 'var(--safety)' },
}

export const chanceMeta = (level) => META[level] ?? META[0]

// tierColor maps abit.Tier (0 none, 1 reach, 2 match, 3 safety) to theme tokens.
export const tierColor = (tier) =>
  ({ 3: 'var(--safety)', 2: 'var(--match)', 1: 'var(--reach)' }[tier] ?? 'var(--muted)')
