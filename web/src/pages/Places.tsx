// Places: where you watch the sky from, with how tonight looks at each. Make
// one home, say how dark its sky is, choose which ones send a message on a
// clear night, take places over from Pocket Weather or Pocket Places.
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useData } from '../lib/data'
import { choosePlace } from '../lib/place'
import type { DarkSpot, Darkness, Place } from '../lib/types'
import { openAddPlace } from '../components/Layout'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { Card, Confirm, CoreHint, Empty, Field, Loading, Modal, Spinner, Switch, useToast } from '../components/ui'

export const BORTLE = [
  { value: '0', label: 'Not known' },
  { value: '1', label: '1 · Pristine: far from any town' },
  { value: '2', label: '2 · Truly dark: the Milky Way casts shadows' },
  { value: '3', label: '3 · Countryside: a little glow on the horizon' },
  { value: '4', label: '4 · Village edge: the Milky Way still clear' },
  { value: '5', label: '5 · Suburb: the Milky Way faint' },
  { value: '6', label: '6 · Bright suburb: only the brightest Milky Way' },
  { value: '7', label: '7 · Town: no Milky Way, a grey sky' },
  { value: '8', label: '8 · City: only a few hundred stars' },
  { value: '9', label: '9 · City centre: the brightest stars only' },
]

export default function PlacesPage() {
  const { settings } = useApp()
  const { overview } = useData()
  const { toast } = useToast()
  const [edit, setEdit] = useState<Place | null>(null)
  const [del, setDel] = useState<Place | null>(null)
  const [importFrom, setImportFrom] = useState<'' | 'weather' | 'places'>('')
  if (!overview) return <Loading />
  const places = overview.places
  const save = async (p: Place, patch: Partial<Place>, msg?: string) => {
    try { await api.put(`${V1}/locations/${p.id}`, patch); announceChange(); if (msg) toast(msg) } catch (e) { toast((e as Error).message, 'err') }
  }
  const move = async (i: number, dir: number) => {
    const ids = places.map(p => p.id)
    const k = i + dir
    if (k < 0 || k >= ids.length) return
    ;[ids[i], ids[k]] = [ids[k], ids[i]]
    try { await api.post(`${V1}/locations/order`, { ids }); announceChange() } catch (e) { toast((e as Error).message, 'err') }
  }
  const coreOn = !!settings?.core_configured && settings.core_registered !== false
  const can = (slug: 'weather' | 'places') => !!settings?.[`${slug}_configured`]
  return (
    <div className="stack loose">
      <Card title="Your places" flush actions={<button type="button" className="btn primary sm" onClick={openAddPlace}><Icon name="plus" size={14} />Add a place</button>}
        sub="Home is the place Tonight, the map and the evening message use first. A clear night at a place with the bell on sends you a message.">
        {places.length === 0 ? <div className="pad"><Empty title="No places yet" icon="pin"><button type="button" className="link-btn" onClick={openAddPlace}>Add where you watch the sky from</button></Empty></div> : (
          <div className="list">
            {places.map((p, i) => (
              <div key={p.id} className="sky-place">
                <Link to="/" onClick={() => choosePlace(p.id)} className="sky-place-main">
                  <Icon name={p.is_home ? 'home' : 'pin'} size={18} className="muted" />
                  <span className="grow" style={{ minWidth: 0 }}>
                    <span className="row-title">{p.name}{p.is_home && <span className="badge accent">home</span>}{p.source && p.source !== 'import' && <span className="badge">from Pocket {p.source[0].toUpperCase() + p.source.slice(1)}</span>}</span>
                    <span className="muted small block-line ellipsis">{[p.note, p.region, p.country].filter(Boolean).join(' · ')}{p.elevation ? ` · ${Math.round(p.elevation)} m` : ''}{p.bortle ? ` · sky ${p.bortle}/9` : ''}</span>
                  </span>
                  {p.tonight && <span className={`pill tone-${p.tonight.score.label.toLowerCase()}`} title={p.tonight.score.words}>{p.tonight.score.forecast ? `${p.tonight.score.label} tonight` : 'No forecast'}</span>}
                </Link>
                <span className="sky-place-ctl">
                  <span className="row small muted" title="A message when a clear, dark night is coming here"><Switch size="sm" checked={p.alerts} onChange={v => save(p, { alerts: v }, v ? 'Clear nights here will send a message' : 'No messages for this place')} label={`Clear-night messages for ${p.name}`} /><Icon name="bell" size={14} /></span>
                  <button type="button" className="btn ghost sm icon-only" aria-label="Move up" disabled={i === 0} onClick={() => move(i, -1)}><Icon name="chevron-up" size={14} /></button>
                  <button type="button" className="btn ghost sm icon-only" aria-label="Move down" disabled={i === places.length - 1} onClick={() => move(i, 1)}><Icon name="chevron-down" size={14} /></button>
                  <button type="button" className="btn ghost sm icon-only" aria-label="Edit" onClick={() => setEdit(p)}><Icon name="edit" size={14} /></button>
                  <button type="button" className="btn ghost sm icon-only danger-text" aria-label="Remove" onClick={() => setDel(p)}><Icon name="trash" size={14} /></button>
                </span>
              </div>
            ))}
          </div>
        )}
      </Card>
      {places.length > 0 && <DarkerNearby />}
      <Card title="From your other apps" sub="Take over places you already keep elsewhere: only their names and where they are.">
        <div className="row wrap" style={{ gap: 8 }}>
          {(['weather', 'places'] as const).map(slug => (
            <button key={slug} type="button" className="btn" disabled={!can(slug)} title={can(slug) ? undefined : coreOn ? `Switch the Sky → Pocket ${slug === 'weather' ? 'Weather' : 'Places'} link on in Pocket Core first` : 'Needs Pocket Core, or a direct connection in Settings → Connected apps'}
              onClick={() => setImportFrom(slug)}><Icon name={slug === 'weather' ? 'cloud' : 'pin'} size={15} />From Pocket {slug === 'weather' ? 'Weather' : 'Places'}</button>
          ))}
        </div>
        {!coreOn && <CoreHint className="mt" />}
        {coreOn && (!can('weather') || !can('places')) && <p className="muted small mt">Not connected yet: see <Link to="/settings/apps">Settings → Connected apps</Link>.</p>}
      </Card>
      {edit && <EditPlace p={edit} onClose={() => setEdit(null)} onSave={async patch => { await save(edit, patch, 'Saved'); setEdit(null) }} />}
      {importFrom && <ImportPlaces from={importFrom} onClose={() => setImportFrom('')} />}
      {del && <Confirm confirmLabel="Remove" title={`Remove ${del.name}?`} message="Your log entries from there keep its name." onCancel={() => setDel(null)}
        onConfirm={async () => { const d = del; setDel(null); try { await api.del(`${V1}/locations/${d.id}`); announceChange(); toast(`${d.name} removed`) } catch (e) { toast((e as Error).message, 'err') } }} />}
    </div>
  )
}

function EditPlace({ p, onClose, onSave }: { p: Place; onClose: () => void; onSave: (patch: Partial<Place>) => void }) {
  const [f, setF] = useState({ name: p.name, note: p.note, is_home: p.is_home, bortle: p.bortle })
  return (
    <Modal title={`Edit ${p.name}`} onClose={onClose} size="sm">
      <form onSubmit={e => { e.preventDefault(); onSave(f) }} className="stack">
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} required maxLength={80} autoFocus /></Field>
        <Field label="How dark is the sky there?" help="The Bortle scale: it decides which faint sights Tonight suggests. Not sure? A town is about 7, a village 4 or 5, the mountains 2 or 3.">
          <Select value={String(f.bortle)} onChange={v => setF({ ...f, bortle: Number(v) })} options={BORTLE} ariaLabel="Darkness of the sky" />
          <AtlasHint place={p} onUse={b => setF({ ...f, bortle: b })} />
        </Field>
        <Field label="Note" help="For you: “trees to the east”, “park by the lake”."><input value={f.note} onChange={e => setF({ ...f, note: e.target.value })} maxLength={500} /></Field>
        <label className="row" style={{ gap: 8 }}><Switch checked={f.is_home} onChange={v => setF({ ...f, is_home: v })} label="Home" disabled={p.is_home} />This is home</label>
        <p className="muted small">{p.lat.toFixed(4)}, {p.lon.toFixed(4)}{p.timezone ? ` · ${p.timezone}` : ''}</p>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary">Save</button></div>
      </form>
    </Modal>
  )
}

interface ImportRow { id: number; name: string; region: string; country: string; added: boolean; have: boolean }

function ImportPlaces({ from, onClose }: { from: 'weather' | 'places'; onClose: () => void }) {
  const { toast } = useToast()
  const [rows, setRows] = useState<ImportRow[] | null>(null)
  const [err, setErr] = useState('')
  const [pick, setPick] = useState<Set<number>>(new Set())
  const [busy, setBusy] = useState(false)
  const app = from === 'weather' ? 'Pocket Weather' : 'Pocket Places'
  useEffect(() => {
    api.post<{ places: ImportRow[] }>(`${V1}/locations/import`, { from }).then(r => setRows(r.places)).catch(e => setErr((e as Error).message))
  }, [from])
  const go = async () => {
    setBusy(true)
    try {
      const r = await api.post<{ added: number }>(`${V1}/locations/import`, { from, ids: [...pick] })
      toast(`${r.added} ${r.added === 1 ? 'place' : 'places'} added`, 'ok'); announceChange(); onClose()
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={`Places from ${app}`} onClose={onClose} size="sm" footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" disabled={!pick.size || busy} onClick={go}>{busy && <Spinner />}Add {pick.size || ''}</button></>}>
      {err && <p className="bad small">{err}</p>}
      {!rows && !err && <Loading />}
      {rows && rows.length === 0 && <p className="muted">{app} has no places with a spot on the map.</p>}
      {rows && rows.length > 0 && (
        <div className="list bordered">
          {rows.map(r => (
            <label key={r.id} className="list-row">
              <input type="checkbox" disabled={r.have} checked={r.have || pick.has(r.id)} onChange={e => { const n = new Set(pick); if (e.target.checked) n.add(r.id); else n.delete(r.id); setPick(n) }} />
              <span className="row-main grow"><span className="row-title">{r.name}</span><span className="row-sub muted small">{[r.region, r.country].filter(Boolean).join(', ')}{r.have ? ' · already in Sky' : ''}</span></span>
            </label>
          ))}
        </div>
      )}
    </Modal>
  )
}

function AtlasHint({ place, onUse }: { place: Place; onUse: (bortle: number) => void }) {
  const [d, setD] = useState<Darkness | null>(null)
  useEffect(() => { api.get<{ available: boolean; here: Darkness | null }>(`${V1}/darkness?location=${place.id}`).then(r => setD(r.here)).catch(() => {}) }, [place.id])
  if (!d) return null
  return (
    <p className="small muted mt-s row" style={{ gap: 8, flexWrap: 'wrap' }}>
      <span>The light-pollution atlas says zone {d.zone}: {d.words} (about {d.bortle}).</span>
      {place.bortle !== d.bortle && <button type="button" className="link-btn" onClick={() => onUse(d.bortle)}>Use {d.bortle}</button>}
    </p>
  )
}

/** The darkest skies within a drive of one of your places, from the light-pollution atlas. */
function DarkerNearby() {
  const { overview } = useData()
  const { toast } = useToast()
  const places = overview?.places || []
  const [from, setFrom] = useState(() => String(places.find(p => p.is_home)?.id || places[0]?.id || ''))
  const [km, setKm] = useState(50)
  const [res, setRes] = useState<{ available: boolean; detail?: string; here?: Darkness | null; spots?: DarkSpot[] } | null>(null)
  const [busy, setBusy] = useState(false)
  const look = async () => {
    setBusy(true)
    try { setRes(await api.get(`${V1}/darkness?location=${from}&near=${km}`)) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const add = async (sp: DarkSpot) => {
    try {
      await api.post(`${V1}/locations`, { name: sp.name ? `Dark sky near ${sp.name}` : `Dark sky ${Math.round(sp.km)} km ${sp.dir_words}`, region: sp.region, lat: sp.lat, lon: sp.lon, bortle: sp.bortle, note: `Light-pollution zone ${sp.zone}` })
      toast('Added to your places', 'ok'); announceChange()
    } catch (e) { toast((e as Error).message, 'err') }
  }
  return (
    <Card title="Darker skies nearby" sub="The darkest places within a drive, from the World Atlas of the Artificial Night Sky Brightness (D. Lorenz, 2025 satellite data). Check that there is a road and a safe place to stop.">
      <div className="row wrap" style={{ gap: 8 }}>
        <Select className="place-picker" value={from} onChange={setFrom} options={places.map(p => ({ value: String(p.id), label: `From ${p.name}` }))} ariaLabel="From" />
        <Select className="km-pick" value={String(km)} onChange={v => setKm(Number(v))} options={[25, 50, 80, 120].map(k => ({ value: String(k), label: `Within ${k} km` }))} ariaLabel="Distance" />
        <button type="button" className="btn" onClick={look} disabled={busy || !from}>{busy ? <Spinner /> : <Icon name="search" size={15} />}Find dark skies</button>
      </div>
      {res && !res.available && <p className="muted small mt">{res.detail}. It downloads by itself in the first minutes after the app starts.</p>}
      {res?.here && <p className="small mt">Here: zone {res.here.zone}, {res.here.words} (about Bortle {res.here.bortle}).</p>}
      {res?.spots && (
        <div className="list bordered mt">
          {res.spots.length === 0 && <p className="muted small pad">Nothing darker within {km} km.</p>}
          {res.spots.map(sp => (
            <div key={`${sp.lat},${sp.lon}`} className="list-row">
              <span className="row-main grow"><span className="row-title">{sp.name ? `Near ${sp.name}` : 'Unnamed spot'} <span className="badge">zone {sp.zone}</span></span>
                <span className="row-sub small muted">{sp.km} km {sp.dir_words} · {sp.words} · about Bortle {sp.bortle}</span></span>
              <a className="btn ghost sm" href={`https://www.openstreetmap.org/?mlat=${sp.lat}&mlon=${sp.lon}#map=12/${sp.lat}/${sp.lon}`} target="_blank" rel="noreferrer">Map</a>
              <button type="button" className="btn sm" onClick={() => add(sp)}><Icon name="plus" size={14} />Add</button>
            </div>
          ))}
        </div>
      )}
    </Card>
  )
}
