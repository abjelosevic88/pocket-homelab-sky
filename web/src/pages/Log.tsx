// The observing log: every night out under the sky and what you saw, with
// how the sky was. Log a night in a few taps; it ticks off your lists.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useData, useOnChange, announceChange } from '../lib/data'
import { fmtDate, fmtHours, localAt, nightOf } from '../lib/format'
import { placeQuery, usePlace } from '../lib/place'
import type { ObsObject, Observation, Tonight } from '../lib/types'
import { useCrumb } from '../components/Layout'
import { Icon } from '../components/icons'
import { MoonGlyph } from '../components/SkyBits'
import { ObjectPicker } from '../components/ObjectPicker'
import { Select } from '../components/Select'
import { Card, Confirm, Empty, Field, Loading, Modal, Segmented, Spinner, useDebounce, useToast } from '../components/ui'

export const openLog = () => window.dispatchEvent(new CustomEvent('psky:log'))

export default function LogPage() {
  useCrumb(undefined)
  const { id } = useParams()
  const navigate = useNavigate()
  const [sp, setSp] = useSearchParams()
  const [q, setQ] = useState('')
  const dq = useDebounce(q.trim(), 250)
  const [list, setList] = useState<Observation[] | null>(null)
  const [err, setErr] = useState('')
  const [editing, setEditing] = useState<Observation | 'new' | null>(null)
  const [prefill, setPrefill] = useState<string>('')
  const load = useCallback(() => {
    api.get<Observation[]>(`${V1}/observations${dq ? `?q=${encodeURIComponent(dq)}` : ''}`).then(r => { setList(r); setErr('') }).catch(e => setErr((e as Error).message))
  }, [dq])
  useEffect(() => { load() }, [load])
  useOnChange(load)
  // /log?add=jupiter, the FAB and "Log a night" open the form
  useEffect(() => {
    const add = sp.get('add')
    if (add !== null) { setPrefill(add); setEditing('new'); const n = new URLSearchParams(sp); n.delete('add'); setSp(n, { replace: true }) }
  }, [sp, setSp])
  useEffect(() => { const h = () => { setPrefill(''); setEditing('new') }; window.addEventListener('psky:log', h); return () => window.removeEventListener('psky:log', h) }, [])
  useEffect(() => {
    if (!id) return
    api.get<Observation>(`${V1}/observations/${id}`).then(o => setEditing(o)).catch(() => navigate('/log', { replace: true }))
  }, [id, navigate])
  const months = useMemo(() => {
    const m = new Map<string, Observation[]>()
    for (const o of list || []) {
      const k = new Date(o.night + 'T12:00:00').toLocaleDateString(undefined, { month: 'long', year: 'numeric' })
      if (!m.has(k)) m.set(k, [])
      m.get(k)!.push(o)
    }
    return [...m.entries()]
  }, [list])
  const close = () => { setEditing(null); if (id) navigate('/log', { replace: true }) }
  return (
    <div className="stack loose">
      <div className="row wrap" style={{ gap: 8 }}>
        <div className="home-search grow" style={{ maxWidth: 420 }}><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder="Search the log: Saturn, Bjelašnica, dew…" aria-label="Search the log" /></div>
        <span className="spacer" />
        <button type="button" className="btn primary" onClick={() => { setPrefill(''); setEditing('new') }}><Icon name="plus" size={15} />Log a night</button>
      </div>
      {err && <div className="callout danger"><Icon name="alert" size={15} /><span>{err}</span></div>}
      {!list && !err && <Loading />}
      {list && list.length === 0 && <Empty title={dq ? 'Nothing found' : 'No nights logged yet'} icon="journal">{dq ? 'Try another word.' : 'After a night out, note what you saw. It ticks off your lists and Pocket AI learns what you like to look at.'}</Empty>}
      {months.map(([month, rows]) => (
        <Card key={month} title={month} flush>
          <div className="list">
            {rows.map(o => (
              <button key={o.id} type="button" className="log-row" onClick={() => navigate(`/log/${o.id}`)}>
                <span className="log-date"><b>{new Date(o.night + 'T12:00:00').getDate()}</b><span className="muted small">{new Date(o.night + 'T12:00:00').toLocaleDateString(undefined, { weekday: 'short' })}</span></span>
                <span className="grow" style={{ minWidth: 0 }}>
                  <span className="row-title">{o.title || `Night of ${fmtDate(o.night, false)}`}{o.rating > 0 && <span className="stars small" aria-label={`${o.rating} of 5`}>{'★'.repeat(o.rating)}<span className="muted">{'★'.repeat(5 - o.rating)}</span></span>}</span>
                  <span className="row-sub small muted ellipsis">{[o.place, o.equipment, o.started_at && o.ended_at ? fmtHours((new Date(o.ended_at).getTime() - new Date(o.started_at).getTime()) / 3600000) : ''].filter(Boolean).join(' · ')}</span>
                  {o.objects.length > 0 && <span className="log-objs">{o.objects.slice(0, 8).map(x => <span key={x.key} className="obj-chip small">{x.name}</span>)}{o.objects.length > 8 && <span className="muted small">+{o.objects.length - 8}</span>}</span>}
                </span>
                {o.moon_phase != null && <MoonGlyph phase={o.moon_phase} waxing size={16} />}
              </button>
            ))}
          </div>
        </Card>
      ))}
      {editing && <LogForm o={editing === 'new' ? null : editing} prefill={prefill} onClose={close} onSaved={() => { close(); load() }} />}
    </div>
  )
}

const RATING = [1, 2, 3, 4, 5]

export function LogForm({ o, prefill, onClose, onSaved }: { o: Observation | null; prefill?: string; onClose: () => void; onSaved: () => void }) {
  const { settings } = useApp()
  const { overview } = useData()
  const { place } = usePlace()
  const { toast } = useToast()
  const places = overview?.places || []
  const tz = place?.timezone
  const tonight = nightOf(Date.now(), tz)
  const nowLocal = localAt(Date.now(), tz)
  const [f, setF] = useState(() => ({
    night: o?.night || tonight,
    location_id: o?.location_id ?? (place?.id || 0),
    start: o?.started_at?.slice(11, 16) || (o ? '' : nowLocal.slice(11, 16)),
    end: o?.ended_at?.slice(11, 16) || '',
    title: o?.title || '', notes: o?.notes || '', equipment: o?.equipment || '',
    seeing: o?.seeing || 0, transparency: o?.transparency || 0, cloud: o?.cloud ?? null as number | null, rating: o?.rating || 0,
  }))
  const [objs, setObjs] = useState<ObsObject[]>(o?.objects || [])
  const [kit, setKit] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [del, setDel] = useState(false)
  const [up, setUp] = useState<{ key: string; name: string }[]>([])
  useEffect(() => { api.get<string[]>(`${V1}/equipment`).then(setKit).catch(() => {}) }, [])
  // a new night starts at the place the pages show (it may load after the form opens)
  useEffect(() => { if (!o && place?.id) setF(v => (v.location_id ? v : { ...v, location_id: place.id! })) }, [o, place?.id])
  // prefill from the map ("Log it")
  useEffect(() => {
    if (!prefill || o) return
    api.get<{ key: string; name: string }>(`${V1}/objects/${encodeURIComponent(prefill)}?${placeQuery(place)}`).then(x => setObjs(v => v.some(y => y.key === x.key) ? v : [...v, { key: x.key, name: x.name, note: '' }])).catch(() => {})
  }, [prefill]) // eslint-disable-line react-hooks/exhaustive-deps
  // what was up that night, to add in one tap
  useEffect(() => {
    const p = places.find(x => x.id === f.location_id)
    const q = p ? `location=${p.id}` : placeQuery(place)
    if (!q) return
    api.get<Tonight>(`${V1}/tonight?${q}&date=${f.night}`).then(t => {
      const list = [{ key: 'moon', name: 'Moon' }, ...t.planets.filter(x => x.visible).map(x => ({ key: x.key, name: x.name })), ...t.sights.slice(0, 8).map(x => ({ key: x.key, name: x.name }))]
      setUp(t.moon.up_dark > 0.05 && t.moon.phase > 0.05 ? list : list.slice(1))
    }).catch(() => setUp([]))
  }, [f.location_id, f.night]) // eslint-disable-line react-hooks/exhaustive-deps
  const add = (x: { key: string; name: string }) => setObjs(v => v.some(y => (y.key || y.name) === (x.key || x.name)) ? v : [...v, { key: x.key, name: x.name, note: '' }])
  const timeOf = (hhmm: string) => {
    if (!hhmm) return ''
    // times after midnight belong to the next calendar day
    const d = new Date(f.night + 'T12:00:00Z')
    if (Number(hhmm.slice(0, 2)) < 12) d.setUTCDate(d.getUTCDate() + 1)
    return d.toISOString().slice(0, 10) + 'T' + hhmm
  }
  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    const body = {
      night: f.night, location_id: f.location_id || 0, started_at: timeOf(f.start), ended_at: timeOf(f.end), title: f.title, notes: f.notes, equipment: f.equipment,
      seeing: f.seeing, transparency: f.transparency, cloud: f.cloud ?? -1, rating: f.rating, objects: objs.map(x => ({ key: x.key, name: x.name, note: x.note })),
    }
    try {
      if (o) await api.put(`${V1}/observations/${o.id}`, body)
      else await api.post(`${V1}/observations`, body)
      toast(o ? 'Saved' : 'Night logged', 'ok'); announceChange(); onSaved()
    } catch (er) { toast((er as Error).message, 'err') } finally { setBusy(false) }
  }
  const toNotes = async () => {
    if (!o) return
    try { await api.post(`${V1}/observations/${o.id}/note`); toast('Saved in Pocket Notes (folder Sky)', 'ok') } catch (er) { toast((er as Error).message, 'err') }
  }
  const notesOn = !!settings?.notes_configured
  const placeOptions = [{ value: '0', label: 'No saved place' }, ...places.map(p => ({ value: String(p.id), label: p.name }))]
  return (
    <Modal title={o ? 'This night' : 'Log a night'} onClose={onClose} size="lg"
      footer={<>
        {o && <button type="button" className="btn ghost danger-text" onClick={() => setDel(true)}><Icon name="trash" size={14} />Delete</button>}
        {o && <button type="button" className="btn ghost" disabled={!notesOn} title={notesOn ? 'Save this night in Pocket Notes' : 'Connect Pocket Notes in Settings → Connected apps first'} onClick={toNotes}><Icon name="note" size={14} />Save as a note</button>}
        <span className="spacer" />
        <button type="button" className="btn" onClick={onClose}>Cancel</button>
        <button type="submit" form="log-form" className="btn primary" disabled={busy}>{busy && <Spinner />}{o ? 'Save' : 'Log it'}</button>
      </>}>
      <form id="log-form" onSubmit={save} className="stack">
        <div className="grid-3">
          <Field label="Night of"><input type="date" value={f.night} onChange={e => setF({ ...f, night: e.target.value })} required /></Field>
          <Field label="From"><input type="time" value={f.start} onChange={e => setF({ ...f, start: e.target.value })} /></Field>
          <Field label="Until"><input type="time" value={f.end} onChange={e => setF({ ...f, end: e.target.value })} /></Field>
        </div>
        <div className="grid-2">
          <Field label="Where"><Select value={String(f.location_id || 0)} onChange={v => setF({ ...f, location_id: Number(v) })} options={placeOptions} ariaLabel="Place" /></Field>
          <Field label="With">
            <input list="kit" value={f.equipment} onChange={e => setF({ ...f, equipment: e.target.value })} placeholder="Eyes, 10×50 binoculars, 8-inch telescope…" maxLength={200} />
            <datalist id="kit">{[...kit, 'Eyes', '10×50 binoculars', 'Small telescope'].filter((v, i, a) => a.indexOf(v) === i).map(k => <option key={k} value={k} />)}</datalist>
          </Field>
        </div>
        <Field label="Title" help="Optional: “First look at Saturn's rings”."><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} maxLength={200} /></Field>
        <Field label="What you saw" group>
          <ObjectPicker onPick={add} />
          {up.length > 0 && <div className="chips mt-s" aria-label="Up that night">{up.filter(x => !objs.some(y => y.key === x.key)).slice(0, 10).map(x => <button key={x.key} type="button" className="chip" onClick={() => add(x)}><Icon name="plus" size={12} />{x.name}</button>)}</div>}
          {objs.length > 0 && (
            <div className="list bordered mt-s">
              {objs.map((x, i) => (
                <div key={(x.key || x.name) + i} className="obj-row">
                  <span className="row-title">{x.name}</span>
                  <input className="grow" value={x.note} onChange={e => setObjs(v => v.map((y, j) => j === i ? { ...y, note: e.target.value } : y))} placeholder="A note: what it looked like" maxLength={2000} aria-label={`Note for ${x.name}`} />
                  <button type="button" className="btn ghost sm icon-only" aria-label={`Remove ${x.name}`} onClick={() => setObjs(v => v.filter((_, j) => j !== i))}><Icon name="x" size={14} /></button>
                </div>
              ))}
            </div>
          )}
        </Field>
        <div className="grid-2">
          <Field label="Steady air (seeing)" help="1 stars boil, 5 rock steady"><Segmented size="sm" value={f.seeing} onChange={v => setF({ ...f, seeing: v })} options={[0, ...RATING].map(n => ({ value: n, label: n ? String(n) : '–' }))} label="Seeing" /></Field>
          <Field label="Clear air (transparency)" help="1 hazy, 5 crystal clear"><Segmented size="sm" value={f.transparency} onChange={v => setF({ ...f, transparency: v })} options={[0, ...RATING].map(n => ({ value: n, label: n ? String(n) : '–' }))} label="Transparency" /></Field>
        </div>
        <div className="grid-2">
          <Field label="Cloud"><Segmented size="sm" value={f.cloud == null ? -1 : f.cloud} onChange={v => setF({ ...f, cloud: v < 0 ? null : v })} options={[{ value: -1, label: '–' }, { value: 0, label: 'Clear' }, { value: 25, label: 'Some' }, { value: 50, label: 'Half' }, { value: 75, label: 'Mostly' }]} label="Cloud" /></Field>
          <Field label="How good was the night?"><Segmented size="sm" value={f.rating} onChange={v => setF({ ...f, rating: v })} options={[0, ...RATING].map(n => ({ value: n, label: n ? '★'.repeat(n) : '–' }))} label="Rating" /></Field>
        </div>
        <Field label="Notes"><textarea rows={4} value={f.notes} onChange={e => setF({ ...f, notes: e.target.value })} maxLength={20000} placeholder="What you saw, who came, what to try next time." /></Field>
      </form>
      {del && o && <Confirm title="Delete this night?" message="What you saw that night goes with it." confirmLabel="Delete" onCancel={() => setDel(false)}
        onConfirm={async () => { setDel(false); try { await api.del(`${V1}/observations/${o.id}`); toast('Deleted'); announceChange(); onSaved() } catch (er) { toast((er as Error).message, 'err') } }} />}
    </Modal>
  )
}
