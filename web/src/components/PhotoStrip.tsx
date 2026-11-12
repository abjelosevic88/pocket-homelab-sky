// Photos kept with a night of the log, and "what's in this photo?".
import { useCallback, useEffect, useRef, useState } from 'react'
import { api, V1 } from '../lib/api'
import { Icon } from './icons'
import { Confirm, Modal, Spinner, useToast } from './ui'

interface Annotation { names: string[]; type: string; x: number; y: number; radius: number }
interface Solve { engine: 'astrometry' | 'ai'; state: 'queued' | 'solving' | 'success' | 'failure'; error?: string; text?: string; objects?: string[]; result?: { calibration: { ra: number; dec: number; radius: number; pixscale: number }; objects: string[]; annotations: Annotation[] } }
export interface Photo { id: number; observation_id: number | null; key: string; caption: string; mime: string; width: number; height: number; url: string; solve: Solve | null; created_at: string }

export function PhotoStrip({ observationId }: { observationId: number }) {
  const { toast } = useToast()
  const [list, setList] = useState<Photo[]>([])
  const [busy, setBusy] = useState(false)
  const [open, setOpen] = useState<Photo | null>(null)
  const input = useRef<HTMLInputElement>(null)
  const load = useCallback(() => { api.get<Photo[]>(`${V1}/photos?observation=${observationId}`).then(setList).catch(() => {}) }, [observationId])
  useEffect(() => { load() }, [load])
  const upload = async (files: FileList | null) => {
    if (!files?.length) return
    setBusy(true)
    for (const f of Array.from(files)) {
      const fd = new FormData()
      fd.append('file', f); fd.append('observation_id', String(observationId))
      const res = await fetch(`${V1}/photos`, { method: 'POST', body: fd, credentials: 'same-origin' })
      if (!res.ok) { const e = await res.json().catch(() => ({ error: res.statusText })); toast(`${f.name}: ${e.error}`, 'err') }
    }
    setBusy(false); load()
  }
  return (
    <div className="photo-strip">
      {list.map(p => <button key={p.id} type="button" className="photo-thumb" onClick={() => setOpen(p)} aria-label={p.caption || 'Photo'}><img src={p.url} alt={p.caption} loading="lazy" />{p.solve?.state === 'success' && <span className="photo-badge"><Icon name="check" size={11} /></span>}</button>)}
      <button type="button" className="photo-add" onClick={() => input.current?.click()} disabled={busy}>{busy ? <Spinner /> : <Icon name="camera" size={18} />}<span className="small">Add photos</span></button>
      <input ref={input} type="file" accept="image/*" multiple hidden onChange={e => { upload(e.target.files); e.target.value = '' }} />
      {open && <PhotoViewer photo={open} onClose={() => { setOpen(null); load() }} />}
    </div>
  )
}

export function PhotoViewer({ photo, onClose }: { photo: Photo; onClose: () => void }) {
  const { toast } = useToast()
  const [p, setP] = useState(photo)
  const [del, setDel] = useState(false)
  const [labels, setLabels] = useState(true)
  const [box, setBox] = useState({ w: 0, h: 0 })
  const img = useRef<HTMLImageElement>(null)
  const pending = p.solve && (p.solve.state === 'queued' || p.solve.state === 'solving')
  useEffect(() => {
    if (!pending) return
    const id = window.setInterval(() => { api.get<Photo>(`${V1}/photos/${p.id}`).then(setP).catch(() => {}) }, 6000)
    return () => window.clearInterval(id)
  }, [pending, p.id])
  const solve = async (engine?: 'ai') => {
    try { setP(await api.post<Photo>(`${V1}/photos/${p.id}/solve${engine ? '?engine=ai' : ''}`)) } catch (e) { toast((e as Error).message, 'err') }
  }
  const measure = () => { const el = img.current; if (el) setBox({ w: el.clientWidth, h: el.clientHeight }) }
  const r = p.solve?.result
  const sx = p.width ? box.w / p.width : 0
  return (
    <Modal title={p.caption || 'Photo'} onClose={onClose} size="xl" footer={<>
      <button type="button" className="btn ghost danger-text" onClick={() => setDel(true)}><Icon name="trash" size={14} />Delete</button>
      <span className="spacer" />
      {r && <label className="row small" style={{ gap: 6 }}><input type="checkbox" checked={labels} onChange={e => setLabels(e.target.checked)} />Names on the photo</label>}
      <button type="button" className="btn" onClick={() => solve()} disabled={!!pending}>{pending ? <Spinner /> : <Icon name="sparkles" size={14} />}What's in this photo?</button>
    </>}>
      <div className="photo-view">
        <div className="photo-frame">
          <img ref={img} src={p.url} alt={p.caption} onLoad={measure} />
          {r && labels && sx > 0 && r.annotations.slice(0, 60).map((a, i) => (
            <div key={i} className={`photo-ann ${a.type}`} style={{ left: a.x * sx, top: a.y * sx, width: Math.max(14, a.radius * 2 * sx), height: Math.max(14, a.radius * 2 * sx) }}>
              <span>{a.names[0]}</span>
            </div>
          ))}
        </div>
        {p.solve?.state === 'success' && p.solve.engine === 'astrometry' && r && <p className="small">Pointing at RA {(r.calibration.ra / 15).toFixed(2)} h, Dec {r.calibration.dec.toFixed(1)}°, {(r.calibration.radius * 2).toFixed(1)}° across. In the picture: {r.objects.slice(0, 20).join(', ') || 'stars only'}.</p>}
        {p.solve?.state === 'success' && p.solve.engine === 'ai' && <p className="small">{p.solve.text}{p.solve.objects?.length ? ` (${p.solve.objects.join(', ')})` : ''} <span className="muted">— Pocket AI's reading; for exact names add an astrometry.net key in Settings → AI.</span></p>}
        {pending && <p className="small muted"><Spinner /> astrometry.net is matching the stars (often a minute or two)…</p>}
        {p.solve?.state === 'failure' && <p className="small bad">{p.solve.error} {p.solve.engine === 'astrometry' && <button type="button" className="link-btn" onClick={() => solve('ai')}>Ask Pocket AI instead</button>}</p>}
      </div>
      {del && <Confirm title="Delete this photo?" message="It is removed from Pocket Sky." confirmLabel="Delete" onCancel={() => setDel(false)} onConfirm={async () => { await api.del(`${V1}/photos/${p.id}`); onClose() }} />}
    </Modal>
  )
}
