// Small pictures: the Moon's phase, the night's score, an event's icon.
import type { EventKind } from '../lib/types'
import { Icon } from './icons'

/** The Moon as it looks: lit fraction 0–1, lit on the right while waxing (from the north). */
export function MoonGlyph({ phase, waxing, size = 20 }: { phase: number; waxing: boolean; size?: number }) {
  const r = 10
  const k = Math.max(0, Math.min(1, phase))
  const rx = Math.abs(r * (1 - 2 * k))
  const sweep = k < 0.5 ? 0 : 1
  const lit = k < 0.02 ? '' : k > 0.98 ? `M0 ${-r}A${r} ${r} 0 1 1 0 ${r}A${r} ${r} 0 1 1 0 ${-r}Z` : `M0 ${-r}A${r} ${r} 0 0 1 0 ${r}A${rx} ${r} 0 0 ${sweep} 0 ${-r}Z`
  return (
    <svg className="moon-glyph" width={size} height={size} viewBox="-11 -11 22 22" aria-label={`Moon ${Math.round(k * 100)}% lit`} role="img">
      <circle r={r} className="moon-dark" />
      {lit && <path d={lit} className="moon-lit" transform={waxing ? undefined : 'scale(-1 1)'} />}
      <circle r={r} className="moon-rim" />
    </svg>
  )
}

/** The night's score in a ring. */
export function ScoreRing({ value, label, size = 76 }: { value: number; label: string; size?: number }) {
  const r = 30
  const c = 2 * Math.PI * r
  return (
    <svg className={`score-ring tone-${label.toLowerCase()}`} width={size} height={size} viewBox="0 0 76 76" role="img" aria-label={`${label}, ${value} of 100`}>
      <circle cx="38" cy="38" r={r} className="ring-bg" />
      <circle cx="38" cy="38" r={r} className="ring-fg" strokeDasharray={`${(c * value) / 100} ${c}`} transform="rotate(-90 38 38)" />
      <text x="38" y="44" textAnchor="middle" className="ring-num">{value}</text>
    </svg>
  )
}

const ICON: Record<EventKind, string> = {
  meteor_shower: 'sparkles', lunar_eclipse: 'moon', solar_eclipse: 'sun', conjunction: 'constellation', opposition: 'planet', elongation: 'planet', moon_phase: 'moon', season: 'sunrise',
}

export function EventIcon({ kind, sub }: { kind: EventKind; sub?: string }) {
  return <span className={`ev-ico ev-${kind} ${sub ? 'ev-' + sub : ''}`}><Icon name={ICON[kind] || 'star'} size={15} /></span>
}

export const EVENT_LABEL: Record<EventKind, string> = {
  meteor_shower: 'Meteor showers', lunar_eclipse: 'Eclipses of the Moon', solar_eclipse: 'Eclipses of the Sun', conjunction: 'Close meetings',
  opposition: 'Planets at their best', elongation: 'Mercury and Venus at their best', moon_phase: 'Moon phases', season: 'Equinoxes and solstices',
}
