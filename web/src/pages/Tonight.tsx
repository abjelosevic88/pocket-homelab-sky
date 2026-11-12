// Tonight: is it worth going out? How clear and dark, when, the Moon, the
// planets and the best sights, the coming nights and events — and Pocket AI
// planning the night in plain words.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useOnChange } from '../lib/data'
import { fmtAt, fmtDate, fmtDayAt, fmtHours, nightOf } from '../lib/format'
import { placeQuery, usePlace } from '../lib/place'
import type { NightHour, NightSummary, PlanetView, SightView, SkyEvent, Tonight as TonightView } from '../lib/types'
import { openAddPlace, useCrumb } from '../components/Layout'
import { Icon } from '../components/icons'
import { MoonGlyph, ScoreRing, EventIcon } from '../components/SkyBits'
import { PlacePicker } from '../components/PlacePicker'
import { Card, Loading, Spinner, useToast } from '../components/ui'

const addDays = (d: string, n: number) => { const t = new Date(d + 'T12:00:00Z'); t.setUTCDate(t.getUTCDate() + n); return t.toISOString().slice(0, 10) }

export default function Tonight() {
  useCrumb(undefined)
  const { place, loaded, locate, locating } = usePlace()
  const [date, setDate] = useState('')
  const [data, setData] = useState<TonightView | null>(null)
  const [err, setErr] = useState('')
  const q = placeQuery(place)
  const load = useCallback(() => {
    if (!q) return
    api.get<TonightView>(`${V1}/tonight?${q}${date ? `&date=${date}` : ''}`).then(d => { setData(d); setErr('') }).catch(e => setErr((e as Error).message))
  }, [q, date])
  useEffect(() => { setData(null); load() }, [load])
  useOnChange(load)
  useEffect(() => { const t = window.setInterval(() => { if (document.visibilityState === 'visible') load() }, 20 * 60 * 1000); return () => window.clearInterval(t) }, [load])

  if (loaded && !place) return <Welcome onLocate={locate} locating={locating} />
  const tz = place?.timezone
  const today = place ? nightOf(Date.now(), tz) : ''
  const shown = data?.date || date || today
  return (
    <div className="stack loose sky-tonight">
      <div className="row tonight-head">
        <PlacePicker />
        <span className="spacer" />
        <div className="row" style={{ gap: 4 }}>
          <button type="button" className="btn ghost sm icon-only" aria-label="The night before" onClick={() => setDate(addDays(shown, -1))}><Icon name="chevron-left" size={16} /></button>
          <span className="night-date">{shown === today ? 'Tonight' : shown === addDays(today, 1) ? 'Tomorrow night' : `Night of ${fmtDate(shown, false)}`}</span>
          <button type="button" className="btn ghost sm icon-only" aria-label="The next night" onClick={() => setDate(addDays(shown, 1))}><Icon name="chevron-right" size={16} /></button>
          {shown !== today && <button type="button" className="btn sm" onClick={() => setDate('')}>Tonight</button>}
        </div>
      </div>
      {err && <div className="callout danger"><Icon name="alert" size={15} /><span>{err}</span></div>}
      {!data && !err && <Loading />}
      {data && <>
        <Hero v={data} tz={tz} />
        <NightChart hours={data.hours} tz={tz} />
        <Nights q={q} tz={tz} current={data.date} onPick={d => setDate(d === today ? '' : d)} />
        <Plan q={q} date={data.date} />
        <div className="grid-2">
          <Planets list={data.planets} tz={tz} />
          <Sights list={data.sights} tz={tz} />
        </div>
        <Coming events={data.events} tz={tz} />
      </>}
    </div>
  )
}

function Welcome({ onLocate, locating }: { onLocate: () => void; locating: boolean }) {
  return (
    <div className="welcome card">
      <div className="welcome-body">
        <img src="/favicon.svg" alt="" width={56} height={56} />
        <div className="col">
          <h2>Where do you watch the sky from?</h2>
          <p className="muted">Add your home first. Then a dark spot outside town, the cottage, wherever you go to look up. Pocket Sky tells you which nights will be clear and dark there, what is up, and when.</p>
          <div className="row wrap mt" style={{ gap: 8 }}>
            <button type="button" className="btn primary" onClick={openAddPlace}><Icon name="plus" size={15} />Add a place</button>
            <button type="button" className="btn" onClick={onLocate} disabled={locating}><Icon name="locate" size={15} />{locating ? 'Finding you…' : 'Just use where I am'}</button>
          </div>
        </div>
      </div>
    </div>
  )
}

function Hero({ v, tz }: { v: TonightView; tz?: string }) {
  const n = v.night
  const s = v.score
  const facts: [string, string, string, string?][] = [
    ['sunset', 'Sunset', fmtAt(n.sunset, tz), n.civil_dusk ? `twilight to ${fmtAt(n.civil_dusk, tz)}` : undefined],
    ['moon', 'Dark from', n.dark_kind === 'none' ? '—' : fmtAt(n.dark_from, tz), n.dark_kind === 'astro' ? 'fully dark' : n.dark_kind === 'nautical' ? 'never fully dark' : n.dark_kind === 'civil' ? 'stays bright' : undefined],
    ['sunrise', 'Dark until', n.dark_kind === 'none' ? '—' : fmtAt(n.dark_to, tz), n.sunrise ? `sunrise ${fmtAt(n.sunrise, tz)}` : undefined],
    ['clock', 'Dark hours', n.dark_kind === 'none' ? '—' : fmtHours(n.dark_hours), n.moon_dark_hrs < n.dark_hours - 0.2 ? `${fmtHours(n.moon_dark_hrs)} without the Moon` : undefined],
    ['cloud', 'Cloud', s.cloud_avg == null ? '—' : `${Math.round(s.cloud_avg)} %`, s.cloud_avg == null ? 'no forecast yet' : s.clear ? 'clear enough' : 'too cloudy'],
  ]
  return (
    <section className={`card tn-hero tone-${s.label.toLowerCase()}`}>
      <div className="tn-hero-main">
        <ScoreRing value={s.value} label={s.label} />
        <div className="col grow" style={{ minWidth: 0 }}>
          <div className="tn-hero-title">{s.label === 'Cloudy' ? 'A cloudy night' : `${s.label} night for the sky`}{v.stale && <span className="badge warn" title="The forecast service did not answer; this is the last forecast Sky has">older forecast</span>}</div>
          <p className="tn-hero-words">{s.words}</p>
          <div className="tn-moon-line">
            <MoonGlyph phase={v.moon.phase} waxing={v.moon.waxing} size={22} />
            <span>{v.moon.words}</span>
          </div>
        </div>
      </div>
      <dl className="tn-facts">
        {facts.map(([icon, label, value, sub]) => (
          <div key={label} className="tn-fact">
            <dt><Icon name={icon} size={14} />{label}</dt>
            <dd className="tnum">{value}</dd>
            {sub && <dd className="muted small">{sub}</dd>}
          </div>
        ))}
      </dl>
    </section>
  )
}

function useWidth(fallback: number) {
  const [w, setW] = useState(fallback)
  const ro = useRef<ResizeObserver | null>(null)
  const ref = useCallback((el: Element | null) => {
    ro.current?.disconnect(); ro.current = null
    if (!el) return
    const measure = () => { const cw = Math.round(el.getBoundingClientRect().width); if (cw > 0) setW(cw) }
    measure(); ro.current = new ResizeObserver(measure); ro.current.observe(el)
  }, [])
  return [ref, w] as const
}

/** The night hour by hour: how dark (shaded), the cloud (bars) and when the Moon is up. */
function NightChart({ hours, tz }: { hours: NightHour[]; tz?: string }) {
  const [ref, W] = useWidth(640)
  const H = 150
  const L = 34, R = 10, T = 16, B = 26
  const n = hours.length
  const hasCloud = hours.some(h => h.cloud != null)
  const geo = useMemo(() => {
    const iw = Math.max(40, W - L - R)
    const bw = n > 0 ? iw / n : iw
    return { iw, bw }
  }, [W, n])
  if (n === 0) return null
  const ih = H - T - B
  const every = Math.max(1, Math.ceil(n / Math.max(2, Math.floor(geo.iw / 56))))
  const shade = (sun: number) => sun < -18 ? 'var(--sky-dark)' : sun < -12 ? 'var(--sky-astro)' : sun < -6 ? 'var(--sky-naut)' : sun < 0 ? 'var(--sky-civil)' : 'var(--sky-day)'
  return (
    <Card title="Through the night" sub={hasCloud ? 'Cloud by the hour over how dark the sky is; the yellow line is the Moon above the horizon.' : 'How dark the sky gets; no cloud forecast for this night yet.'} flush>
      <div className="tn-chart" ref={ref}>
        <svg viewBox={`0 0 ${W} ${H}`} width={W} height={H} role="img" aria-label="Cloud and darkness through the night">
          {hours.map((h, i) => <rect key={'s' + i} x={L + i * geo.bw} y={T} width={geo.bw + 0.5} height={ih} fill={shade(h.sun_alt)} />)}
          {[0, 50, 100].map(p => <g key={p}>
            <line x1={L} x2={W - R} y1={T + ih * (1 - p / 100)} y2={T + ih * (1 - p / 100)} className="tn-grid" />
            <text x={L - 6} y={T + ih * (1 - p / 100) + 4} textAnchor="end" className="tn-axis">{p}%</text>
          </g>)}
          {hasCloud && hours.map((h, i) => h.cloud == null ? null : (
            <rect key={'c' + i} x={L + i * geo.bw + Math.min(3, geo.bw * 0.15)} width={Math.max(2, geo.bw - Math.min(6, geo.bw * 0.3))}
              y={T + ih * (1 - h.cloud / 100)} height={Math.max(1, ih * h.cloud / 100)} rx={Math.min(3, geo.bw / 4)} className="tn-cloud" />
          ))}
          {hours.map((h, i) => h.moon_alt > 0 ? <rect key={'m' + i} x={L + i * geo.bw} y={T - 6} width={geo.bw + 0.5} height={3} rx={1.5} className="tn-moonbar" /> : null)}
          {hours.map((h, i) => i % every === 0 ? <text key={'x' + i} x={L + i * geo.bw + geo.bw / 2} y={H - 8} textAnchor="middle" className="tn-axis">{fmtAt(h.time, tz)}</text> : null)}
        </svg>
      </div>
      <div className="tn-legend small muted">
        <span><i className="sw dark" />Dark</span><span><i className="sw twi" />Twilight</span>{hasCloud && <span><i className="sw cloud" />Cloud</span>}<span><i className="sw moon" />Moon up</span>
      </div>
    </Card>
  )
}

function Nights({ q, tz, current, onPick }: { q: string; tz?: string; current: string; onPick: (d: string) => void }) {
  const [list, setList] = useState<NightSummary[] | null>(null)
  useEffect(() => { let a = true; api.get<{ nights: NightSummary[] }>(`${V1}/nights?${q}`).then(r => { if (a) setList(r.nights) }).catch(() => {}); return () => { a = false } }, [q])
  if (!list) return null
  return (
    <Card title="The coming nights" flush>
      <div className="tn-nights" role="tablist" aria-label="The coming nights">
        {list.map((n, i) => (
          <button key={n.date} type="button" role="tab" aria-selected={n.date === current} className={`tn-night ${n.date === current ? 'on' : ''} tone-${n.score.label.toLowerCase()}`} onClick={() => onPick(n.date)}>
            <span className="tn-night-day">{i === 0 ? 'Tonight' : fmtDayAt(n.date + 'T12:00:00Z', 'UTC').split(' ')[0]}</span>
            <span className="tn-night-label">{n.score.forecast ? n.score.label : '—'}</span>
            <span className="tn-night-bar"><span style={{ width: `${n.score.value}%` }} /></span>
            <span className="tn-night-sub muted small"><MoonGlyph phase={n.moon_phase} waxing={n.moon_name.startsWith('Wax') || n.moon_name === 'First quarter'} size={12} />{n.score.cloud_avg != null ? `${Math.round(n.score.cloud_avg)}%` : fmtHours(n.dark_hours)}</span>
          </button>
        ))}
      </div>
      <p className="muted small" style={{ padding: '0 16px 12px' }}>Cloud is the average over the dark hours.</p>
    </Card>
  )
}

function Plan({ q, date }: { q: string; date: string }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const [state, setState] = useState<{ busy: boolean; text: string; error: string }>({ busy: false, text: '', error: '' })
  const [ready, setReady] = useState<boolean | null>(null)
  const ask = useCallback(async (fresh = false) => {
    setState(s => ({ ...s, busy: true, error: '' }))
    const loc = new URLSearchParams(q)
    try {
      const r = await api.post<{ ok: boolean; text?: string; error?: string }>(`${V1}/ai/plan?${q}`, { location_id: Number(loc.get('location')) || 0, date, fresh })
      setState({ busy: false, text: r.ok ? r.text || '' : '', error: r.ok ? '' : r.error || 'Pocket AI did not answer' })
    } catch (e) { setState({ busy: false, text: '', error: (e as Error).message }); toast((e as Error).message, 'err') }
  }, [q, date, toast])
  useEffect(() => { api.get<{ ready: boolean }>(`${V1}/ai/status`).then(r => setReady(r.ready)).catch(() => setReady(false)) }, [])
  useEffect(() => { setState({ busy: false, text: '', error: '' }); if (ready && settings?.ai_plan) ask() }, [ready, settings?.ai_plan, ask])
  if (ready === null) return null
  return (
    <Card title={<span className="row" style={{ gap: 6 }}><Icon name="sparkles" size={15} />The night in plain words</span>}
      actions={ready ? <button type="button" className="btn ghost sm" onClick={() => ask(true)} disabled={state.busy}>{state.busy ? <Spinner /> : <Icon name="refresh" size={14} />}{state.text ? 'Again' : 'Plan my night'}</button> : undefined}>
      {!ready && <p className="muted small">Pocket AI can plan the night for you: what to look at, when and where, for what you have. Set it up in <Link to="/settings/ai">Settings → AI</Link>.</p>}
      {ready && state.busy && !state.text && <p className="muted small"><Spinner /> Looking at the sky for you…</p>}
      {ready && state.text && <p className="tn-plan">{state.text}</p>}
      {ready && !state.text && !state.busy && !state.error && <p className="muted small">Ask Pocket AI what to look at tonight, in what order, and when.</p>}
      {state.error && <p className="bad small">{state.error}</p>}
    </Card>
  )
}

const PLANET_DOT: Record<string, string> = { mercury: '#b9a48e', venus: '#e8c66a', mars: '#e5683f', jupiter: '#d9a96a', saturn: '#cdb35e', uranus: '#6cc6d2', neptune: '#5a7be8' }

function Planets({ list, tz }: { list: PlanetView[]; tz?: string }) {
  const navigate = useNavigate()
  const vis = list.filter(p => p.visible)
  const rest = list.filter(p => !p.visible)
  return (
    <Card title="Planets" sub={vis.length ? `${vis.length} up during the night` : 'None well placed tonight'} flush>
      <div className="list">
        {vis.map(p => (
          <button key={p.key} type="button" className="list-row" onClick={() => navigate(`/map?object=${p.key}`)}>
            <span className="planet-dot" style={{ background: PLANET_DOT[p.key] }} />
            <span className="row-main grow"><span className="row-title">{p.name} <span className="muted small">in {p.con_name}</span></span><span className="row-sub small">{p.words}</span></span>
            <span className="tnum small muted">{fmtAt(p.best_at, tz)}</span>
          </button>
        ))}
        {rest.length > 0 && <div className="list-foot muted small">Not tonight: {rest.map(p => `${p.name} (${p.words.replace(/\.$/, '').toLowerCase()})`).join('; ')}.</div>}
      </div>
    </Card>
  )
}

const KIT: Record<string, [string, string]> = { eyes: ['eye', 'With your eyes'], binoculars: ['search', 'Binoculars'], telescope: ['telescope', 'Telescope'] }

function Sights({ list, tz }: { list: SightView[]; tz?: string }) {
  const navigate = useNavigate()
  return (
    <Card title="Best sights tonight" sub="Clusters, nebulae and galaxies well placed for how dark it gets" flush>
      {list.length === 0 && <p className="muted small" style={{ padding: 16 }}>Nothing much tonight: the sky does not get dark enough.</p>}
      <div className="list">
        {list.map(s => (
          <button key={s.key} type="button" className="list-row" onClick={() => navigate(`/map?object=${encodeURIComponent(s.key)}`)}>
            <Icon name={KIT[s.kit][0]} size={16} className="muted" />
            <span className="row-main grow"><span className="row-title">{s.name}{s.seen && <Icon name="check-circle" size={13} className="ok" />}</span><span className="row-sub small muted">{s.type} in {s.con_name} · {KIT[s.kit][1]}</span></span>
            <span className="tnum small muted">{fmtAt(s.best_at, tz)} {s.dir}</span>
          </button>
        ))}
      </div>
    </Card>
  )
}

function Coming({ events, tz }: { events: SkyEvent[]; tz?: string }) {
  if (events.length === 0) return null
  return (
    <Card title="Coming up this week" actions={<Link to="/events" className="btn ghost sm">All events<Icon name="chevron-right" size={14} /></Link>} flush>
      <div className="list">
        {events.slice(0, 8).map(e => (
          <Link key={e.key} to={`/events#${encodeURIComponent(e.key)}`} className="list-row">
            <EventIcon kind={e.kind} sub={e.sub} />
            <span className="row-main grow"><span className="row-title">{e.title}</span><span className="row-sub small muted ellipsis">{e.detail}</span></span>
            <span className="tnum small muted">{fmtDayAt(e.t, tz)} {fmtAt(e.t, tz)}</span>
          </Link>
        ))}
      </div>
    </Card>
  )
}
