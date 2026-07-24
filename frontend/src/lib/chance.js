// Maps the abit.ChanceLevel int (analysis.chance) to display metadata,
// mirroring ChanceLevel.Emoji()/Label() on the Go side.
const META = {
  0: { emoji: '❔', label: 'Невідомий', color: '#6b7280' },
  1: { emoji: '⚫', label: 'Нульовий', color: '#374151' },
  2: { emoji: '🔴', label: 'Низький', color: '#dc2626' },
  3: { emoji: '🟡', label: 'Середній', color: '#d97706' },
  4: { emoji: '🟢', label: 'Високий', color: '#16a34a' },
  5: { emoji: '🟢', label: 'Високий (Квота 1)', color: '#16a34a' },
  6: { emoji: '🟢', label: 'Високий (Квота 2)', color: '#16a34a' },
}

export const chanceMeta = (level) => META[level] ?? META[0]

// tierColor maps abit.Tier (0 none, 1 reach, 2 match, 3 safety).
export const tierColor = (tier) => ({ 3: '#16a34a', 2: '#d97706', 1: '#dc2626' }[tier] ?? '#6b7280')
