// Find something in the sky by name (planets, stars, Messier objects,
// constellations), or type your own ("a bright satellite").
import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import type { SkyObject } from '../lib/types'
import { Icon } from './icons'
import { useDebounce } from './ui'

export function ObjectPicker({ onPick, placeholder = 'Add what you saw: Jupiter, M31, Orion…', allowOther = true }: { onPick: (o: { key: string; name: string }) => void; placeholder?: string; allowOther?: boolean }) {
  const [q, setQ] = useState('')
  const dq = useDebounce(q.trim(), 180)
  const [res, setRes] = useState<SkyObject[]>([])
  const [hl, setHl] = useState(0)
  useEffect(() => {
    if (!dq) { setRes([]); return }
    let alive = true
    api.get<SkyObject[]>(`${V1}/objects/search?q=${encodeURIComponent(dq)}&limit=8`).then(r => { if (alive) { setRes(r); setHl(0) } }).catch(() => {})
    return () => { alive = false }
  }, [dq])
  const choose = (o: { key: string; name: string }) => { onPick(o); setQ(''); setRes([]) }
  const other = allowOther && q.trim().length > 1 && !res.some(r => r.name.toLowerCase() === q.trim().toLowerCase())
  const rows = [...res.map(r => ({ key: r.key, name: r.name, sub: r.type })), ...(other ? [{ key: '', name: q.trim(), sub: 'Your own' }] : [])]
  return (
    <div className="obj-picker">
      <div className="home-search">
        <Icon name="search" size={15} />
        <input value={q} onChange={e => setQ(e.target.value)} placeholder={placeholder} aria-label="Find in the sky"
          onKeyDown={e => {
            if (e.key === 'ArrowDown') { e.preventDefault(); setHl(h => Math.min(rows.length - 1, h + 1)) }
            if (e.key === 'ArrowUp') { e.preventDefault(); setHl(h => Math.max(0, h - 1)) }
            if (e.key === 'Enter' && rows[hl]) { e.preventDefault(); choose(rows[hl]) }
          }} />
      </div>
      {q.trim() && rows.length > 0 && (
        <div className="obj-results">
          {rows.map((r, i) => (
            <button key={r.key || 'other'} type="button" className={`sky-result ${i === hl ? 'hl' : ''}`} onMouseEnter={() => setHl(i)} onClick={() => choose(r)}>
              <span className="grow ellipsis">{r.key ? r.name : <>Add “{r.name}”</>}</span><span className="muted small">{r.sub}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
