// Lists: what you want to see, and how far along you are with the Messier
// list, the planets and the constellations. Ticked off by the log.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useOnChange } from '../lib/data'
import { fmtDate } from '../lib/format'
import { placeQuery, usePlace } from '../lib/place'
import type { Progress, ProgressGroup, Target } from '../lib/types'
import { useCrumb } from '../components/Layout'
import { Icon } from '../components/icons'
import { ObjectPicker } from '../components/ObjectPicker'
import { Card, Empty, Loading, Switch, Tabs, useToast } from '../components/ui'

type Tab = 'want' | 'messier' | 'planets' | 'constellations' | 'favourites'

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
      ]} />
      {!prog && <Loading />}
      {tab === 'want' && <Want targets={targets} onChange={load} />}
      {prog && tab === 'messier' && <Group g={prog.messier} title="The Messier list" sub="110 clusters, nebulae and galaxies Charles Messier listed in the 1700s: the classic list to work through with binoculars and a small telescope." />}
      {prog && tab === 'planets' && <Group g={prog.planets} title="Planets and the Moon" sub="Uranus needs binoculars and a chart; Neptune a telescope." />}
      {prog && tab === 'constellations' && <Group g={prog.constellations} title="The 88 constellations" sub="Log a constellation when you have found its shape in the sky." />}
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
