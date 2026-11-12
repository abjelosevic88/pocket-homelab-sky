// Sky events for the coming year at your place: eclipses, meteor showers,
// the Moon passing the planets, planets at their best, the Moon's phases and
// the seasons. Each can go on your calendar.
import { useEffect, useMemo, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { fmtAt, fmtDayAt, localAt } from '../lib/format'
import { placeQuery, usePlace } from '../lib/place'
import type { EventKind, SkyEvent } from '../lib/types'
import { useCrumb } from '../components/Layout'
import { Icon } from '../components/icons'
import { EVENT_LABEL, EventIcon } from '../components/SkyBits'
import { PlacePicker } from '../components/PlacePicker'
import { Card, Empty, Loading, Switch, Tabs, useToast } from '../components/ui'
import { SatPasses } from '../components/SatPasses'

const KINDS: EventKind[] = ['meteor_shower', 'lunar_eclipse', 'solar_eclipse', 'conjunction', 'opposition', 'elongation', 'moon_phase', 'season']

export default function Events() {
  useCrumb(undefined)
  const { place } = usePlace()
  const { settings } = useApp()
  const { toast } = useToast()
  const navigate = useNavigate()
  const loc = useLocation()
  const [events, setEvents] = useState<SkyEvent[] | null>(null)
  const [err, setErr] = useState('')
  const [kinds, setKinds] = useState<Set<EventKind>>(() => new Set(KINDS.filter(k => k !== 'moon_phase')))
  const [onlyVisible, setOnlyVisible] = useState(true)
  const [tab, setTab] = useState<'events' | 'sats'>(() => (loc.hash === '#satellites' ? 'sats' : 'events'))
  const q = placeQuery(place)
  useEffect(() => {
    if (!q) return
    setEvents(null)
    api.get<{ events: SkyEvent[] }>(`${V1}/events?${q}`).then(r => { setEvents(r.events); setErr('') }).catch(e => setErr((e as Error).message))
  }, [q])
  useEffect(() => {
    const id = decodeURIComponent(loc.hash.slice(1))
    if (!id || !events) return
    const el = document.getElementById('ev-' + id)
    if (el) { el.scrollIntoView({ block: 'center' }); el.classList.add('flash') }
  }, [loc.hash, events])
  const shown = useMemo(() => (events || []).filter(e => kinds.has(e.kind) && (!onlyVisible || e.visible !== false)), [events, kinds, onlyVisible])
  const months = useMemo(() => {
    const m = new Map<string, SkyEvent[]>()
    for (const e of shown) {
      const k = new Date(e.t).toLocaleDateString(undefined, { month: 'long', year: 'numeric', timeZone: place?.timezone || undefined })
      if (!m.has(k)) m.set(k, [])
      m.get(k)!.push(e)
    }
    return [...m.entries()]
  }, [shown, place?.timezone])
  const calendarOn = !!settings?.calendar_configured
  const addToCalendar = async (e: SkyEvent) => {
    const tz = place?.timezone
    const start = e.start ? localAt(e.start, tz) : e.kind === 'meteor_shower' ? localAt(new Date(e.t).getTime(), tz) : localAt(e.t, tz)
    const end = e.end ? localAt(e.end, tz) : ''
    try {
      await api.post(`${V1}/calendar`, { title: e.title, description: e.detail + '\n\nFrom Pocket Sky', start, end, location: place?.name || '' })
      toast('On your calendar', 'ok')
    } catch (er) { toast((er as Error).message, 'err') }
  }
  return (
    <div className="stack loose">
      <div className="row wrap" style={{ gap: 8 }}>
        <PlacePicker />
        <Tabs value={tab} onChange={setTab} tabs={[{ id: 'events', label: 'Sky events', icon: 'calendar' }, { id: 'sats', label: 'Satellites', icon: 'star' }]} />
        <span className="spacer" />
        {tab === 'events' && <label className="row small" style={{ gap: 8 }}><Switch size="sm" checked={onlyVisible} onChange={setOnlyVisible} label="Only what I can see" />Only what can be seen from here</label>}
      </div>
      {tab === 'sats' && <Card title="Satellites passing over" sub="The space stations, the brightest satellites and new Starlink trains you can see from here in the next three days: when, where to look and how bright." flush><SatPasses q={q} tz={place?.timezone} /></Card>}
      {tab === 'events' && <>
      <div className="chips" role="group" aria-label="Kinds of events">
        {KINDS.map(k => (
          <button key={k} type="button" className={`chip ${kinds.has(k) ? 'on' : ''}`} aria-pressed={kinds.has(k)} onClick={() => { const n = new Set(kinds); if (n.has(k)) n.delete(k); else n.add(k); setKinds(n) }}>
            <EventIcon kind={k} />{EVENT_LABEL[k]}
          </button>
        ))}
      </div>
      {err && <div className="callout danger"><Icon name="alert" size={15} /><span>{err}</span></div>}
      {!events && !err && <Loading label="Working the sky out for the year…" />}
      {events && shown.length === 0 && <Empty title="Nothing to show" icon="calendar">Switch on more kinds of events above.</Empty>}
      {months.map(([month, list]) => (
        <Card key={month} title={month} flush>
          <div className="list">
            {list.map(e => (
              <div key={e.key} id={'ev-' + e.key} className={`ev-row ${e.visible === false ? 'faded' : ''}`}>
                <EventIcon kind={e.kind} sub={e.sub} />
                <div className="grow" style={{ minWidth: 0 }}>
                  <div className="row-title">{e.title}{e.visible === false && <span className="badge">not from here</span>}{e.kind.includes('eclipse') && e.visible && <span className="badge accent">seen from here</span>}</div>
                  <div className="small muted">{fmtDayAt(e.t, place?.timezone, true)} · {e.start && e.end ? `${fmtAt(e.start, place?.timezone)}–${fmtAt(e.end, place?.timezone)}` : fmtAt(e.t, place?.timezone)}{e.kind === 'conjunction' && e.value ? ` · ${e.value}° apart` : ''}{e.kind === 'meteor_shower' && e.value ? ` · up to ${e.value} an hour` : ''}</div>
                  <p className="small ev-detail">{e.detail}</p>
                </div>
                <div className="ev-actions">
                  {(e.bodies?.length ?? 0) > 0 && e.visible !== false && <button type="button" className="btn ghost sm icon-only" title="Show on the map" aria-label="Show on the map" onClick={() => navigate(`/map?object=${e.bodies![e.bodies!.length - 1]}`)}><Icon name="map" size={15} /></button>}
                  <button type="button" className="btn ghost sm icon-only" disabled={!calendarOn} title={calendarOn ? 'Add to Pocket Calendar' : 'Connect Pocket Calendar in Settings → Connected apps first'} aria-label="Add to calendar" onClick={() => addToCalendar(e)}><Icon name="calendar-plus" size={15} /></button>
                </div>
              </div>
            ))}
          </div>
        </Card>
      ))}
      </>}
    </div>
  )
}
