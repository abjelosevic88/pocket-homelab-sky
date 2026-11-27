// Observing plans (what to look at on a night, in the best order, to print
// or take into SkySafari or Stellarium) and your equipment.
import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useData } from '../lib/data'
import { fmtAt, fmtDate, nightOf } from '../lib/format'
import { type Kit, kitWords, setups } from '../lib/kit'
import { usePlace } from '../lib/place'
import type { Target, Tonight } from '../lib/types'
import { useCrumb } from '../components/Layout'
import { Icon } from '../components/icons'
import { ObjectPicker } from '../components/ObjectPicker'
import { Select } from '../components/Select'
import { Card, Confirm, Empty, Field, Loading, MenuButton, Modal, Spinner, Tabs, useToast } from '../components/ui'

interface PlanItem { id?: number; key: string; name: string; note: string; done: boolean; type?: string; mag?: number | null; con_name?: string; best_at?: string | null; best_alt?: number; dir?: string; words?: string }
interface Plan { id: number; name: string; night: string; location_id: number | null; setup: string; notes: string; items: PlanItem[]; place?: { timezone: string; name: string } }

export default function PlansPage() {
  const { id } = useParams()
  // /plans?tab=kit opens on the equipment (what a search result links to)
  const [sp] = useSearchParams()
  const [tab, setTab] = useState<'plans' | 'kit'>(() => sp.get('tab') === 'kit' ? 'kit' : (localStorage.getItem('psky-plans-tab') as 'plans' | 'kit') || 'plans')
  useCrumb(id ? 'Plan' : undefined)
  if (id) return <PlanEditor id={Number(id)} />
  return (
    <div className="stack loose">
      <Tabs value={tab} onChange={t => { setTab(t); localStorage.setItem('psky-plans-tab', t) }} tabs={[{ id: 'plans', label: 'Observing plans', icon: 'list' }, { id: 'kit', label: 'Your equipment', icon: 'telescope' }]} />
      {tab === 'plans' ? <PlanList /> : <KitList />}
    </div>
  )
}

function PlanList() {
  const navigate = useNavigate()
  const { toast } = useToast()
  const { place } = usePlace()
  const [list, setList] = useState<Plan[] | null>(null)
  useEffect(() => { api.get<Plan[]>(`${V1}/plans`).then(setList).catch(e => toast((e as Error).message, 'err')) }, [toast])
  const create = async (withTonight: boolean) => {
    try {
      let items: { key: string }[] = []
      if (withTonight && place) {
        const t = await api.get<Tonight>(`${V1}/tonight?${place.id ? `location=${place.id}` : `lat=${place.lat}&lon=${place.lon}`}`)
        items = [...t.planets.filter(p => p.visible).map(p => ({ key: p.key })), ...t.sights.slice(0, 8).map(s => ({ key: s.key }))]
      }
      const p = await api.post<Plan>(`${V1}/plans`, { name: withTonight ? "Tonight's best" : 'Observing plan', night: nightOf(Date.now(), place?.timezone), location_id: place?.id || 0, items })
      if (items.length) await api.post(`${V1}/plans/${p.id}/order`)
      navigate(`/plans/${p.id}`)
    } catch (e) { toast((e as Error).message, 'err') }
  }
  if (!list) return <Loading />
  return (
    <Card title="Observing plans" sub="A list of things to look at on a night, in the best order, with the time and direction for each. Print it, follow it as a tour on the map, or take it into SkySafari or Stellarium." flush
      actions={<span className="row" style={{ gap: 6 }}><button type="button" className="btn sm" onClick={() => create(true)}><Icon name="sparkles" size={14} />Tonight's best</button><button type="button" className="btn primary sm" onClick={() => create(false)}><Icon name="plus" size={14} />New plan</button></span>}>
      {list.length === 0 && <div className="pad"><Empty title="No plans yet" icon="list">Start with Tonight's best and change it as you like.</Empty></div>}
      <div className="list">
        {list.map(p => (
          <button key={p.id} type="button" className="list-row" onClick={() => navigate(`/plans/${p.id}`)}>
            <Icon name="list" size={16} className="muted" />
            <span className="row-main grow"><span className="row-title">{p.name}</span><span className="row-sub small muted">Night of {fmtDate(p.night)} · {p.items.length} {p.items.length === 1 ? 'thing' : 'things'}{p.setup ? ` · ${p.setup}` : ''}</span></span>
            <Icon name="chevron-right" size={15} className="muted" />
          </button>
        ))}
      </div>
    </Card>
  )
}

function PlanEditor({ id }: { id: number }) {
  const navigate = useNavigate()
  const { toast } = useToast()
  const { overview } = useData()
  const [p, setP] = useState<Plan | null>(null)
  const [kit, setKit] = useState<Kit[]>([])
  const [busy, setBusy] = useState(false)
  const [del, setDel] = useState(false)
  const load = useCallback(() => { api.get<Plan>(`${V1}/plans/${id}`).then(setP).catch(e => toast((e as Error).message, 'err')) }, [id, toast])
  useEffect(() => { load(); api.get<Kit[]>(`${V1}/kit`).then(setKit).catch(() => {}) }, [load])
  useCrumb(p?.name, 'plans')
  if (!p) return <Loading />
  const tz = p.place?.timezone
  const save = async (patch: Partial<Plan> & { items?: PlanItem[] }) => {
    const body: Record<string, unknown> = { ...patch }
    if (patch.items) { body.items = patch.items.map(i => ({ key: i.key, name: i.name, note: i.note })); body.done = patch.items.filter(i => i.done).map(i => i.key) }
    try { setP(await api.put<Plan>(`${V1}/plans/${id}`, body)) } catch (e) { toast((e as Error).message, 'err') }
  }
  const add = (o: { key: string; name: string }) => { if (!p.items.some(i => i.key === o.key)) save({ items: [...p.items, { key: o.key, name: o.name, note: '', done: false }] }) }
  const order = async () => { setBusy(true); try { setP(await api.post<Plan>(`${V1}/plans/${id}/order`)); toast('In the best order', 'ok') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) } }
  const addWants = async () => {
    const ts = await api.get<Target[]>(`${V1}/targets`)
    const more = ts.filter(t => !t.seen_on && !p.items.some(i => i.key === t.key)).map(t => ({ key: t.key, name: t.name, note: t.note, done: false }))
    if (more.length) save({ items: [...p.items, ...more] }); else toast('Nothing new on your want-to-see list')
  }
  const move = (i: number, d: number) => { const it = [...p.items]; const j = i + d; if (j < 0 || j >= it.length) return;[it[i], it[j]] = [it[j], it[i]]; save({ items: it }) }
  const places = overview?.places || []
  const setupOptions = [{ value: '', label: 'Any' }, ...setups(kit).map(s => ({ value: s.label, label: s.label }))]
  return (
    <div className="stack loose plan-page">
      <Card>
        <div className="grid-3 plan-head">
          <Field label="Name"><input value={p.name} onChange={e => setP({ ...p, name: e.target.value })} onBlur={() => save({ name: p.name })} maxLength={200} /></Field>
          <Field label="Night of"><input type="date" value={p.night} onChange={e => save({ night: e.target.value })} /></Field>
          <Field label="Where"><Select value={String(p.location_id || 0)} onChange={v => save({ location_id: Number(v) || 0 })} options={[{ value: '0', label: 'Home' }, ...places.map(x => ({ value: String(x.id), label: x.name }))]} ariaLabel="Place" /></Field>
        </div>
        <div className="grid-2 mt-s">
          <Field label="With"><Select value={p.setup} onChange={v => save({ setup: v })} options={setupOptions} ariaLabel="Equipment" /></Field>
          <Field label="Notes"><input value={p.notes} onChange={e => setP({ ...p, notes: e.target.value })} onBlur={() => save({ notes: p.notes })} maxLength={20000} placeholder="Bring the red torch and a flask" /></Field>
        </div>
      </Card>
      <Card title={`${p.items.length} ${p.items.length === 1 ? 'thing' : 'things'} to look at`} flush
        actions={<span className="row no-print" style={{ gap: 6, flexWrap: 'wrap' }}>
          <button type="button" className="btn sm" onClick={order} disabled={busy || p.items.length < 2}>{busy ? <Spinner /> : <Icon name="clock" size={14} />}Best order</button>
          <button type="button" className="btn sm" onClick={() => navigate(`/map?plan=${p.id}`)} disabled={!p.items.length}><Icon name="route" size={14} />Tour on the map</button>
          <button type="button" className="btn sm" onClick={() => window.print()}><Icon name="file" size={14} />Print</button>
          <MenuButton label="Export" className="btn sm" align="right" items={[
            { label: 'SkySafari (.skylist)', onSelect: () => { window.location.href = `${V1}/plans/${p.id}/export?format=skysafari` } },
            { label: 'Stellarium observing list (.json)', onSelect: () => { window.location.href = `${V1}/plans/${p.id}/export?format=stellarium` } },
            { label: 'Spreadsheet (.csv)', onSelect: () => { window.location.href = `${V1}/plans/${p.id}/export?format=csv` } },
          ]}><Icon name="download" size={14} />Export</MenuButton>
        </span>}>
        <div className="pad no-print stack">
          <ObjectPicker onPick={add} allowOther={false} placeholder="Add: Saturn, M13, the Double Cluster…" />
          <button type="button" className="link-btn small" onClick={addWants}>Add what is on your want-to-see list</button>
        </div>
        {p.items.length === 0 && <div className="pad"><Empty title="Nothing yet" icon="list">Add something above.</Empty></div>}
        <div className="list">
          {p.items.map((it, i) => (
            <div key={it.key} className={`plan-row ${it.done ? 'done' : ''}`}>
              <input type="checkbox" checked={it.done} onChange={e => save({ items: p.items.map((x, j) => (j === i ? { ...x, done: e.target.checked } : x)) })} aria-label={`Seen ${it.name}`} />
              <span className="plan-time tnum">{it.best_at ? fmtAt(it.best_at, tz) : '—'}</span>
              <span className="row-main grow"><span className="row-title">{it.name} <span className="muted small">{[it.type, it.con_name ? `in ${it.con_name}` : '', it.mag != null ? `mag ${it.mag}` : ''].filter(Boolean).join(' · ')}</span></span>
                <span className="row-sub small muted">{it.words}</span></span>
              <span className="row no-print" style={{ gap: 0 }}>
                <button type="button" className="btn ghost sm icon-only" aria-label="Earlier" disabled={i === 0} onClick={() => move(i, -1)}><Icon name="chevron-up" size={14} /></button>
                <button type="button" className="btn ghost sm icon-only" aria-label="Later" disabled={i === p.items.length - 1} onClick={() => move(i, 1)}><Icon name="chevron-down" size={14} /></button>
                <button type="button" className="btn ghost sm icon-only" aria-label={`Remove ${it.name}`} onClick={() => save({ items: p.items.filter((_, j) => j !== i) })}><Icon name="x" size={14} /></button>
              </span>
            </div>
          ))}
        </div>
      </Card>
      <div className="row no-print"><button type="button" className="btn ghost danger-text" onClick={() => setDel(true)}><Icon name="trash" size={14} />Delete this plan</button></div>
      {del && <Confirm title="Delete this plan?" message="Your log is not touched." confirmLabel="Delete" onCancel={() => setDel(false)} onConfirm={async () => { await api.del(`${V1}/plans/${id}`); navigate('/plans') }} />}
    </div>
  )
}

const KINDS: { value: Kit['kind']; label: string }[] = [{ value: 'binoculars', label: 'Binoculars' }, { value: 'telescope', label: 'Telescope' }, { value: 'eyepiece', label: 'Eyepiece' }, { value: 'camera', label: 'Camera' }]

function KitList() {
  const { toast } = useToast()
  const [list, setList] = useState<Kit[] | null>(null)
  const [edit, setEdit] = useState<Partial<Kit> | null>(null)
  const load = useCallback(() => { api.get<Kit[]>(`${V1}/kit`).then(setList).catch(e => toast((e as Error).message, 'err')) }, [toast])
  useEffect(() => { load() }, [load])
  if (!list) return <Loading />
  const ss = setups(list)
  return (
    <div className="stack loose">
      <Card title="Your equipment" sub="Binoculars, telescopes, eyepieces and cameras: Sky works out what each combination shows, and draws its view as a circle on the map." flush
        actions={<button type="button" className="btn primary sm" onClick={() => setEdit({ kind: 'binoculars', name: '', power: 10, aperture: 50 })}><Icon name="plus" size={14} />Add</button>}>
        {list.length === 0 && <div className="pad"><Empty title="Nothing yet" icon="telescope">Add your binoculars or telescope.</Empty></div>}
        <div className="list">
          {list.map(k => (
            <button key={k.id} type="button" className="list-row" onClick={() => setEdit(k)}>
              <Icon name={k.kind === 'camera' ? 'camera' : k.kind === 'binoculars' ? 'search' : 'telescope'} size={16} className="muted" />
              <span className="row-main grow"><span className="row-title">{k.name} <span className="muted small">{KINDS.find(x => x.value === k.kind)?.label}</span></span><span className="row-sub small muted">{kitWords(k)}</span></span>
            </button>
          ))}
        </div>
      </Card>
      {ss.length > 0 && <Card title="What each setup shows" sub="Choose one in the map's layers menu to see its view drawn on the sky." flush>
        <div className="list">{ss.map(s => <div key={s.id} className="list-row"><span className="row-main grow"><span className="row-title">{s.label}</span>{s.reach && <span className="row-sub small muted">Stars to about magnitude {s.reach}</span>}</span></div>)}</div>
      </Card>}
      {edit && <KitForm k={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); load() }} />}
    </div>
  )
}

function KitForm({ k, onClose, onSaved }: { k: Partial<Kit>; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState<Partial<Kit>>(k)
  const [del, setDel] = useState(false)
  const n = (key: keyof Kit, label: string, help?: string) => (
    <Field label={label} help={help}><input type="number" step="any" min={0} value={(f[key] as number) || ''} onChange={e => setF({ ...f, [key]: Number(e.target.value) })} /></Field>
  )
  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    try { await (k.id ? api.put(`${V1}/kit/${k.id}`, f) : api.post(`${V1}/kit`, f)); toast('Saved', 'ok'); onSaved() } catch (er) { toast((er as Error).message, 'err') }
  }
  return (
    <Modal title={k.id ? k.name : 'Add equipment'} onClose={onClose} size="sm" footer={<>
      {k.id && <button type="button" className="btn ghost danger-text" onClick={() => setDel(true)}><Icon name="trash" size={14} />Remove</button>}
      <span className="spacer" />
      <button type="button" className="btn" onClick={onClose}>Cancel</button><button type="submit" form="kit-form" className="btn primary">Save</button></>}>
      <form id="kit-form" onSubmit={save} className="stack">
        <Field label="Kind"><Select value={f.kind || 'binoculars'} onChange={v => setF({ ...f, kind: v as Kit['kind'] })} options={KINDS} ariaLabel="Kind" /></Field>
        <Field label="Name"><input value={f.name || ''} onChange={e => setF({ ...f, name: e.target.value })} required maxLength={120} placeholder={f.kind === 'telescope' ? '8-inch Dobsonian' : f.kind === 'eyepiece' ? '25 mm Plössl' : f.kind === 'camera' ? 'Canon EOS' : '10×50 binoculars'} /></Field>
        {f.kind === 'binoculars' && <div className="grid-2">{n('power', 'Magnification', 'The first number: 10 in 10×50')}{n('aperture', 'Lens size (mm)', 'The second number: 50 in 10×50')}{n('tfov', 'Field of view (°)', 'Often printed on them; empty = worked out')}</div>}
        {f.kind === 'telescope' && <div className="grid-2">{n('aperture', 'Aperture (mm)', '8 inches = 203 mm')}{n('focal', 'Focal length (mm)')}</div>}
        {f.kind === 'eyepiece' && <div className="grid-2">{n('focal', 'Focal length (mm)')}{n('afov', 'Apparent field (°)', 'Plössl 50, wide 68, ultra-wide 82')}</div>}
        {f.kind === 'camera' && <div className="grid-2">{n('sensor_w', 'Sensor width (mm)', 'Full frame 36, APS-C 23.5')}{n('sensor_h', 'Sensor height (mm)', 'Full frame 24, APS-C 15.6')}{n('focal', 'Lens (mm)', 'Empty when it goes on a telescope')}</div>}
        <Field label="Notes"><input value={f.notes || ''} onChange={e => setF({ ...f, notes: e.target.value })} maxLength={2000} /></Field>
      </form>
      {del && k.id && <Confirm title={`Remove ${k.name}?`} message="Your log keeps what you wrote." confirmLabel="Remove" onCancel={() => setDel(false)} onConfirm={async () => { await api.del(`${V1}/kit/${k.id}`); onSaved() }} />}
    </Modal>
  )
}
