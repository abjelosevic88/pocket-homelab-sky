// The space station, bright satellites and Starlink trains passing over a place.
import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { fmtAt, fmtDayAt } from '../lib/format'
import type { SatPass } from '../lib/types'
import { Icon } from './icons'
import { Empty, Loading, Switch } from './ui'

export function SatPasses({ q, tz, days = 3, compact = false }: { q: string; tz?: string; days?: number; compact?: boolean }) {
  const [list, setList] = useState<SatPass[] | null>(null)
  const [err, setErr] = useState('')
  const [all, setAll] = useState(false)
  const [stations, setStations] = useState(compact)
  useEffect(() => {
    if (!q) return
    setList(null)
    api.get<{ passes: SatPass[] }>(`${V1}/satellites/passes?${q}&days=${days}&visible=1`).then(r => { setList(r.passes); setErr('') }).catch(e => setErr((e as Error).message))
  }, [q, days])
  if (err) return <p className="bad small pad">{err}</p>
  if (!list) return <Loading />
  const shown = list.filter(p => (all || p.mag <= 3) && (!stations || p.group === 'station' || p.train))
  return (
    <div>
      {!compact && <div className="row wrap pad small" style={{ gap: 16 }}>
        <label className="row" style={{ gap: 6 }}><Switch size="sm" checked={stations} onChange={setStations} label="Only space stations and Starlink trains" />Only space stations and Starlink trains</label>
        <label className="row" style={{ gap: 6 }}><Switch size="sm" checked={all} onChange={setAll} label="Faint ones too" />Faint ones too</label>
      </div>}
      {shown.length === 0 && <div className="pad"><Empty title="Nothing to see" icon="star">{compact ? 'No bright satellite passes over tonight.' : 'No visible passes in these days.'}</Empty></div>}
      <div className="list">
        {shown.slice(0, compact ? 5 : 80).map(p => (
          <div key={`${p.norad}-${p.start}`} className="ev-row">
            <span className={`ev-ico ${p.group === 'station' ? 'ev-sat' : ''}`}><Icon name={p.train ? 'route' : 'star'} size={15} /></span>
            <div className="grow" style={{ minWidth: 0 }}>
              <div className="row-title">{p.train ? `Starlink train (${p.count})` : p.name}{p.mag < 0 && <span className="badge accent">bright</span>}</div>
              <div className="small muted">{fmtDayAt(p.visible_from || p.start, tz)} · {fmtAt(p.visible_from || p.start, tz)}–{fmtAt(p.visible_to || p.end, tz)} · up to {p.peak_alt}° · magnitude {p.mag}</div>
              <p className="small ev-detail">{p.words}</p>
            </div>
          </div>
        ))}
      </div>
      <p className="muted small pad">Orbits from CelesTrak, updated every few hours; times can shift by a minute.</p>
    </div>
  )
}
