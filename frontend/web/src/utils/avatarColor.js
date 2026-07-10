// Deterministic soft-tint palette for avatars/chips -- same identity always
// gets the same color, and the hues are spread enough apart to stay
// distinguishable (loosely CVD-aware: alternating warm/cool around the wheel).
const PALETTE = [
  { bg: '#eaf2fc', fg: '#2a5fb0' }, // blue
  { bg: '#e6f7f1', fg: '#0f7a5c' }, // teal
  { bg: '#fdf3df', fg: '#a5680a' }, // amber
  { bg: '#eef0fd', fg: '#4f46e5' }, // indigo
  { bg: '#fce9ef', fg: '#b23a63' }, // rose
  { bg: '#f1eefc', fg: '#6b3fbf' }, // violet
  { bg: '#fdece3', fg: '#c1541f' }, // orange
  { bg: '#e9f4e6', fg: '#3c7a2e' }, // green
]

function hashString(str) {
  let hash = 0
  for (let i = 0; i < str.length; i++) {
    hash = (hash << 5) - hash + str.charCodeAt(i)
    hash |= 0
  }
  return Math.abs(hash)
}

export function colorFor(seed) {
  return PALETTE[hashString(seed) % PALETTE.length]
}

export function initials(name) {
  return name
    .split(' ')
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0].toUpperCase())
    .join('')
}
