// Lists: what you want to see, and how far along you are with the Messier
// list, the planets and the constellations. Ticked off by the log.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useOnChange } from '../lib/data'
import { fmtAt, fmtDate, fmtDayAt } from '../lib/format'
import { registerExtra } from '../lib/sky'
import { placeQuery, usePlace } from '../lib/place'
import type { Progress, ProgressGroup, Target } from '../lib/types'
import { useCrumb } from '../components/Layout'
import { Icon } from '../components/icons'
import { ObjectPicker } from '../components/ObjectPicker'
import { Card, Empty, Loading, Switch, Tabs, useToast } from '../components/ui'

type Tab = 'want' | 'messier' | 'planets' | 'constellations' | 'favourites' | 'doubles'

export default function Lists() {
  useCrumb(undefined)
  const [tab, setTab] = useState<Tab>(() => (localStorage.getItem('psky-lists') as Tab) || 'want')
  const { place } = usePlace()
  const [prog, setProg] = useState<Progress | null>(null)
  const [targets, setTargets] = useState<Target[] | null>(null)
  const { toast } = useToast()
  const q = placeQuery(place)
  const load = useCallback(() => {
    api.get<Progress>(`${V1}/progress?${q}`).then(setProg).catch(e => toast((e as Error).message, 'err'))
    api.get<Target[]>(`${V1}/targets`).then(setTargets).catch(() => {})
  }, [q, toast])
  useEffect(() => { load() }, [load])
  useOnChange(load)
  const pick = (t: Tab) => { setTab(t); localStorage.setItem('psky-lists', t) }
  const st = prog?.stats
  return (
    <div className="stack loose">
      {st && <div className="stat-row">
        <div className="stat"><b className="tnum">{st.nights}</b><span className="muted small">nights logged</span></div>
        <div className="stat"><b className="tnum">{st.objects}</b><span className="muted small">different things seen</span></div>
        <div className="stat"><b className="tnum">{Math.round(st.hours)}</b><span className="muted small">hours under the sky</span></div>
        <div className="stat"><b className="tnum">{prog!.messier.seen}<span className="muted small">/110</span></b><span className="muted small">Messier objects</span></div>
      </div>}
      <Tabs value={tab} onChange={pick} tabs={[
        { id: 'want', label: 'Want to see', icon: 'target' }, { id: 'messier', label: 'Messier list', icon: 'sparkles' },
        { id: 'planets', label: 'Planets and the Moon', icon: 'planet' }, { id: 'constellations', label: 'Constellations', icon: 'constellation' },
        { id: 'favourites', label: 'More favourites', icon: 'star' },
        { id: 'doubles', label: 'Doubles and variables', icon: 'eye' },
      ]} />
      {!prog && <Loading />}
      {tab === 'want' && <Want targets={targets} onChange={load} />}
      {prog && tab === 'messier' && <Group g={prog.messier} title="The Messier list" sub="110 clusters, nebulae and galaxies Charles Messier listed in the 1700s: the classic list to work through with binoculars and a small telescope." />}
      {prog && tab === 'planets' && <Group g={prog.planets} title="Planets and the Moon" sub="Uranus needs binoculars and a chart; Neptune a telescope." />}
      {prog && tab === 'constellations' && <Group g={prog.constellations} title="The 88 constellations" sub="Log a constellation when you have found its shape in the sky." />}
      {tab === 'doubles' && <Doubles q={q} />}
      {prog && tab === 'favourites' && <Group g={prog.highlights} title="More favourites" sub="Well-loved sights outside the Messier list (some only from the south)." />}
    </div>
  )
}

function Want({ targets, onChange }: { targets: Target[] | null; onChange: () => void }) {
  const { toast } = useToast()
  const navigate = useNavigate()
  const add = async (o: { key: string; name: string }) => {
    try { await api.post(`${V1}/targets`, { key: o.key, name: o.name }); onChange(); toast(`${o.name} is on your list`, 'ok') } catch (e) { toast((e as Error).message, 'err') }
  }
  const remove = async (t: Target) => { try { await api.del(`${V1}/targets/${t.id}`); onChange() } catch (e) { toast((e as Error).message, 'err') } }
  if (!targets) return <Loading />
  const open = targets.filter(t => !t.seen_on), done = targets.filter(t => t.seen_on)
  return (
    <Card title="Want to see" sub="Add what you'd like to find. Once it is in your log, it is ticked off here." flush>
      <div className="pad"><ObjectPicker onPick={add} placeholder="Add something: Saturn's rings, M51, the Pleiades…" /></div>
      {targets.length === 0 && <div className="pad"><Empty title="Nothing on your list yet" icon="target">Tap Want to see on anything in the map, or add it above.</Empty></div>}
      <div className="list">
        {[...open, ...done].map(t => (
          <div key={t.id} className={`list-row ${t.seen_on ? 'done' : ''}`}>
            <Icon name={t.seen_on ? 'check-circle' : 'circle'} size={16} className={t.seen_on ? 'ok' : 'muted'} />
            <button type="button" className="row-main grow linkish" onClick={() => navigate(`/map?object=${encodeURIComponent(t.key)}`)}>
              <span className="row-title">{t.name}</span>
              <span className="row-sub small muted">{t.seen_on ? `Seen on ${fmtDate(t.seen_on)}` : t.note || 'Not yet'}{t.seen_on && t.note ? ` · ${t.note}` : ''}</span>
            </button>
            <button type="button" className="btn ghost sm icon-only" aria-label={`Take ${t.name} off the list`} onClick={() => remove(t)}><Icon name="x" size={14} /></button>
          </div>
        ))}
      </div>
    </Card>
  )
}

function Group({ g, title, sub }: { g: ProgressGroup; title: string; sub: string }) {
  const navigate = useNavigate()
  const [tonight, setTonight] = useState(false)
  const [notYet, setNotYet] = useState(false)
  const items = useMemo(() => g.items.filter(i => (!tonight || i.visible_tonight) && (!notYet || !i.seen)), [g, tonight, notYet])
  const pct = g.total ? Math.round((g.seen / g.total) * 100) : 0
  return (
    <Card title={title} sub={sub} flush>
      <div className="pad stack">
        <div className="progress-line"><span className="progress"><span style={{ width: `${pct}%` }} /></span><span className="tnum small">{g.seen} of {g.total}</span></div>
        <div className="row wrap small" style={{ gap: 16 }}>
          <label className="row" style={{ gap: 6 }}><Switch size="sm" checked={tonight} onChange={setTonight} label="Up tonight" />Up tonight</label>
          <label className="row" style={{ gap: 6 }}><Switch size="sm" checked={notYet} onChange={setNotYet} label="Not seen yet" />Not seen yet</label>
        </div>
      </div>
      <div className="prog-grid">
        {items.map(i => (
          <button key={i.key} type="button" className={`prog-item ${i.seen ? 'seen' : ''}`} onClick={() => navigate(`/map?object=${encodeURIComponent(i.key)}`)} title={i.seen ? `First seen ${fmtDate(i.seen)}${i.times > 1 ? `, ${i.times} times` : ''}` : 'Not seen yet'}>
            <Icon name={i.seen ? 'check-circle' : 'circle'} size={14} className={i.seen ? 'ok' : 'muted'} />
            <span className="grow ellipsis"><span className="row-title">{i.name}</span><span className="muted small block-line ellipsis">{i.type}{i.mag != null ? ` · mag ${i.mag}` : ''}</span></span>
            {i.visible_tonight && <span className="dot-up" title="Up tonight" />}
          </button>
        ))}
        {items.length === 0 && <p className="muted small pad">Nothing matches.</p>}
      </div>
    </Card>
  )
}

interface DoubleRow { key: string; name: string; con_name: string; mag1: number; mag2: number; sep_arcsec: number; colors: string; kit: string; about: string; best_at: string | null; best_alt: number; dir: string; seen: string; ra: number; dec: number }
interface VarRow { name: string; key: string; what: string; times: string[] }

function Doubles({ q }: { q: string }) {
  const navigate = useNavigate()
  const { place } = usePlace()
  const [d, setD] = useState<{ doubles: DoubleRow[]; variables: VarRow[] } | null>(null)
  useEffect(() => { api.get<{ doubles: DoubleRow[]; variables: VarRow[] }>(`${V1}/doubles?${q}`).then(r => { setD(r); for (const x of r.doubles) registerExtra(x.key, x.ra, x.dec, false) }).catch(() => {}) }, [q])
  if (!d) return <Loading />
  const tz = place?.timezone
  return (
    <div className="stack loose">
      <Card title="Double stars" sub="Stars that split into two (or more) in binoculars or a small telescope, many in lovely contrasting colours." flush>
        <div className="list">
          {d.doubles.map(x => (
            <button key={x.key} type="button" className="list-row" onClick={() => navigate(`/map?object=${encodeURIComponent(x.key)}`)}>
              <Icon name={x.seen ? 'check-circle' : 'circle'} size={15} className={x.seen ? 'ok' : 'muted'} />
              <span className="row-main grow"><span className="row-title">{x.name} <span className="muted small">in {x.con_name}</span></span>
                <span className="row-sub small muted">{x.colors} · {x.sep_arcsec}″ apart · {x.kit}{x.best_alt > 15 && x.best_at ? ` · best ${fmtAt(x.best_at, tz)} ${x.dir}` : ' · not well placed tonight'}</span></span>
            </button>
          ))}
        </div>
      </Card>
      <Card title="Stars that change" sub="Variable stars you can watch change with your eyes, at times that can be seen from here in the next two weeks.">
        {d.variables.map(v => (
          <div key={v.name} className="mt-s">
            <div className="row-title">{v.name}</div>
            <p className="small muted">{v.name} is {v.what}.</p>
            <p className="small">{v.times.length ? v.times.slice(0, 6).map(t => `${fmtDayAt(t, tz)} ${fmtAt(t, tz)}`).join(' · ') : 'Not at a dark hour from here in the next two weeks.'}</p>
          </div>
        ))}
      </Card>
    </div>
  )
}
