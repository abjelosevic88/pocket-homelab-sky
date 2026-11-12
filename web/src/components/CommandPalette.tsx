// ⌘K / "/" — find anything in the sky, one of your places or a page, or run a command from the keyboard.
import React, { useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useData } from '../lib/data'
import { choosePlace } from '../lib/place'
import type { SkyObject } from '../lib/types'
import { Icon } from './icons'
import { useDebounce } from './ui'

export interface PaletteCommand { id: string; label: string; icon: string; hint?: string; keywords?: string; run: () => void }


export function CommandPalette({ onClose, commands }: { onClose: () => void; commands: PaletteCommand[] }) {
  const navigate = useNavigate()
  const [q, setQ] = useState('')
  const [hi, setHi] = useState(0)
  const list = useRef<HTMLDivElement>(null)
  const { overview } = useData()
  const dq = useDebounce(q.trim(), 160)
  const [sky, setSky] = useState<SkyObject[]>([])
  useEffect(() => {
    if (dq.length < 2) { setSky([]); return }
    let alive = true
    api.get<SkyObject[]>(`${V1}/objects/search?q=${encodeURIComponent(dq)}&limit=6`).then(r => { if (alive) setSky(r) }).catch(() => {})
    return () => { alive = false }
  }, [dq])

  type Row = { key: string; group: string; icon: React.ReactNode; label: React.ReactNode; sub?: React.ReactNode; hint?: string; run: () => void }
  const rows = useMemo<Row[]>(() => {
    const out: Row[] = []
    const n = q.trim().toLowerCase()
    const words = n.split(/\s+/).filter(Boolean)
    const hit = (text: string) => words.every(w => text.toLowerCase().includes(w))
    const cmds = commands.filter(c => !n || hit(`${c.label} ${c.keywords || ''}`)).map(c => ({ key: c.id, group: 'Commands', icon: <Icon name={c.icon} size={16} />, label: c.label, hint: c.hint, run: c.run }))
    const objs = sky.map(o => ({ key: `o${o.key}`, group: 'In the sky', icon: <Icon name={o.kind === 'constellation' ? 'constellation' : o.kind === 'planet' ? 'planet' : o.kind === 'moon' ? 'moon' : o.kind === 'sun' ? 'sun' : 'star'} size={16} />,
      label: o.name, sub: o.type, run: () => navigate(`/map?object=${encodeURIComponent(o.key)}`) }))
    const places = (overview?.places || []).filter(p => !n || hit(`${p.name} ${p.region} ${p.country} ${p.note}`)).map(p => ({
      key: `p${p.id}`, group: 'Your places', icon: <Icon name={p.is_home ? 'home' : 'pin'} size={16} />,
      label: p.name, sub: p.tonight ? `${p.tonight.score.label} tonight` : p.country, run: () => { choosePlace(p.id); navigate('/') },
    }))
    out.push(...objs, ...places, ...cmds)
    return out
  }, [q, overview, commands, navigate, sky])
  useEffect(() => setHi(0), [q])
  useEffect(() => { list.current?.querySelector(`[data-i="${hi}"]`)?.scrollIntoView({ block: 'nearest' }) }, [hi])

  const run = (r: Row | undefined) => { if (!r) return; onClose(); r.run() }
  let last = ''
  return createPortal(
    <div className="palette-backdrop" onMouseDown={e => { if (e.target === e.currentTarget) onClose() }}>
      <div className="palette-box" role="dialog" aria-label="Search and commands">
        <div className="pal-input">
          <Icon name="search" size={18} />
          <input autoFocus value={q} onChange={e => setQ(e.target.value)} placeholder="Find a star, a planet, one of your places, or run a command…" aria-label="Search"
            onKeyDown={e => {
              if (e.key === 'ArrowDown') { e.preventDefault(); setHi(h => Math.min(rows.length - 1, h + 1)) }
              else if (e.key === 'ArrowUp') { e.preventDefault(); setHi(h => Math.max(0, h - 1)) }
              else if (e.key === 'Enter') { e.preventDefault(); run(rows[hi]) }
              else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); onClose() }
            }} />
          <kbd>esc</kbd>
        </div>
        <div className="pal-list" ref={list} role="listbox">
          {rows.length === 0 && <div className="pal-empty">Nothing matches.</div>}
          {rows.map((r, i) => {
            const head = r.group !== last ? <div className="pal-group" key={'g' + r.group}>{r.group}</div> : null
            last = r.group
            return (
              <React.Fragment key={r.key}>
                {head}
                <div data-i={i} role="option" aria-selected={i === hi} className={`pal-row ${i === hi ? 'hi' : ''}`} onMouseEnter={() => setHi(i)} onClick={() => run(r)}>
                  <span className="pal-ico">{r.icon}</span>
                  <span className="grow"><span className="pal-label">{r.label}</span>{r.sub && <span className="pal-sub">{r.sub}</span>}</span>
                  {r.hint && <kbd>{r.hint}</kbd>}
                  {i === hi && <Icon name="chevron-right" size={14} className="muted" />}
                </div>
              </React.Fragment>
            )
          })}
        </div>
      </div>
    </div>, document.body)
}
