// The sky map: drag to look around, pinch or scroll to zoom, tap anything
// to see what it is. "Point at the sky" follows the phone's compass and tilt,
// with the camera picture behind the stars if you like. Time can be moved
// to any hour of any night.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { type BodyKey, BODIES, NAMES, altAz, at, dirShort, direction } from '../lib/astro'
import { fmtAt, fmtDate, fmtDayAt } from '../lib/format'
import { Pointer, askSensors, openCamera, sensorsAvailable } from '../lib/orientation'
import { placeQuery, usePlace } from '../lib/place'
import { type Catalog, type Guide, type Layers, type Painted, directionOf, draw, frameOf, hitTest, loadCatalog, registerExtra, starLabel } from '../lib/sky'
import { MoonMapButton, PlanetDetail } from '../components/SkyDetails'
import { type Kit, type Setup, savedFov, saveFov, setups } from '../lib/kit'
import type { ObjectDetail, SatNow, Settings, SkyObject, Tonight } from '../lib/types'
import { Icon } from '../components/icons'
import { PlacePicker } from '../components/PlacePicker'
import { useCrumb } from '../components/Layout'
import { MenuButton, type MenuItem, Spinner, useDebounce, useToast } from '../components/ui'

type Mode = 'map' | 'point'
const VIEW_KEY = 'psky-view'

function readView() {
  try { const v = JSON.parse(localStorage.getItem(VIEW_KEY) || ''); if (typeof v.az === 'number') return v as { az: number; alt: number; fov: number } } catch { /* first time */ }
  return { az: 180, alt: 35, fov: 90 }
}

export default function SkyMap() {
  useCrumb(undefined)
  const { settings, setSettings } = useApp()
  const { place, loaded, locate } = usePlace()
  const { toast } = useToast()
  const navigate = useNavigate()
  const [sp, setSp] = useSearchParams()
  const [cat, setCat] = useState<Catalog | null>(null)
  const [catErr, setCatErr] = useState('')
  useEffect(() => { loadCatalog().then(setCat).catch(e => setCatErr((e as Error).message)) }, [])

  // ---- time ----
  const [live, setLive] = useState(true)
  const [t, setT] = useState(() => Date.now())
  useEffect(() => {
    if (!live) return
    const id = window.setInterval(() => setT(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [live])
  const [timeOpen, setTimeOpen] = useState(false)
  const [base, setBase] = useState(() => Date.now())

  // ---- the view ----
  const view = useRef(readView())
  const [mode, setMode] = useState<Mode>('map')
  const [camOn, setCamOn] = useState(false)
  const frame = useRef<{ f: [number, number, number]; u: [number, number, number]; r: [number, number, number] } | null>(null)
  const pointer = useRef<Pointer | null>(null)
  const [noSensor, setNoSensor] = useState(false)
  const [northFound, setNorthFound] = useState(true)
  const [camErr, setCamErr] = useState('')
  const [selected, setSelected] = useState<string>('')
  const [selName, setSelName] = useState('')
  const [aim, setAim] = useState<{ alt: number; az: number; name: string; key: string } | null>(null)
  const [path, setPath] = useState('')
  const [guideKey, setGuideKey] = useState('')
  const [guideName, setGuideName] = useState('')
  const [guide, setGuide] = useState<Guide | null>(null)
  const [lineUp, setLineUp] = useState(false)
  const [tour, setTour] = useState<{ steps: TourStep[]; i: number } | null>(null)
  const sats = useRef<{ at: number; list: SatNow[] } | null>(null)
  const [satSel, setSatSel] = useState<SatNow | null>(null)
  const minor = useRef<MinorObj[]>([])
  const [kit, setKit] = useState<Kit[]>([])
  const [fov, setFov] = useState<Setup | null>(savedFov)
  useEffect(() => { api.get<Kit[]>(`${V1}/kit`).then(setKit).catch(() => {}) }, [])
  const [minorSel, setMinorSel] = useState<MinorObj | null>(null)
  const painted = useRef<Painted | null>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const wrap = useRef<HTMLDivElement>(null)
  const video = useRef<HTMLVideoElement>(null)
  const stream = useRef<MediaStream | null>(null)
  const size = useRef({ w: 0, h: 0, dpr: 1 })
  const raf = useRef(0)
  const anim = useRef<{ from: { az: number; alt: number }; to: { az: number; alt: number }; t0: number } | null>(null)

  const layers: Layers = useMemo(() => ({
    lines: settings?.show_lines ?? true, conNames: settings?.show_con_names ?? true, starNames: settings?.show_star_names ?? true, dso: settings?.show_dso ?? true,
    milkyWay: settings?.show_milky_way ?? true, grid: settings?.show_grid ?? false, equator: settings?.show_equator ?? false, ecliptic: settings?.show_ecliptic ?? true,
  }), [settings])
  const night = settings?.night_red ?? false
  const camFov = settings?.camera_fov ?? 64

  const obs = useMemo(() => (place ? { lat: place.lat, lon: place.lon } : null), [place])
  const tz = place?.timezone
  const stateRef = useRef({ t, obs, layers, mode, camOn, selected, magLimit: settings?.mag_limit ?? 5.5, camFov, path, guideKey, tz, live })
  stateRef.current = { t, obs, layers, mode, camOn, selected, magLimit: settings?.mag_limit ?? 5.5, camFov, path, guideKey, tz, live }
  const fovRef = useRef(fov)
  fovRef.current = fov

  const paint = useCallback(() => {
    raf.current = 0
    const c = canvas.current
    const st = stateRef.current
    if (!c || !st.obs) return
    const ctx = c.getContext('2d')
    if (!ctx) return
    const { w, h, dpr } = size.current
    if (!w || !h) return
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    // a running "go to" glide
    if (anim.current) {
      const a = anim.current
      const k = Math.min(1, (performance.now() - a.t0) / 650)
      const e = k < 0.5 ? 2 * k * k : 1 - Math.pow(-2 * k + 2, 2) / 2
      const daz = ((a.to.az - a.from.az + 540) % 360) - 180
      view.current.az = (a.from.az + daz * e + 360) % 360
      view.current.alt = a.from.alt + (a.to.alt - a.from.alt) * e
      if (k >= 1) anim.current = null
      else raf.current = requestAnimationFrame(paint)
    }
    const pointing = st.mode === 'point' && frame.current
    const fr = pointing ? frame.current! : frameOf(view.current.alt, view.current.az)
    const camera = st.mode === 'point' && st.camOn
    const p = draw(ctx, w, h, cat, {
      t: st.t, obs: st.obs, ...fr, fov: camera ? st.camFov : view.current.fov, proj: camera ? 'gnomonic' : 'stereo', fovLong: camera,
      camera, layers: st.layers, magLimit: st.magLimit, selected: st.selected || undefined,
      path: st.path || undefined, guide: st.mode === 'point' && st.guideKey ? st.guideKey : undefined, hourLabel: tt => fmtAt(tt, st.tz),
      sats: st.live && sats.current ? satsAt(sats.current, st.t) : undefined,
      minor: minor.current,
    })
    painted.current = p
    // the field of view of the chosen equipment, at the centre
    const fv = fovRef.current
    if (fv) {
      const half = (deg: number, axis: [number, number, number]) => {
        const a = (deg / 2) * Math.PI / 180
        const e: [number, number, number] = [fr.f[0] * Math.cos(a) + axis[0] * Math.sin(a), fr.f[1] * Math.cos(a) + axis[1] * Math.sin(a), fr.f[2] * Math.cos(a) + axis[2] * Math.sin(a)]
        const q = p.project(e)
        return q ? Math.hypot(q[0] - w / 2, q[1] - h / 2) : 0
      }
      ctx.save()
      ctx.strokeStyle = 'rgba(255, 120, 120, 0.9)'; ctx.lineWidth = 1.5
      ctx.beginPath()
      if (fv.h) { const hw = half(fv.w, fr.r), hh = half(fv.h, fr.u); ctx.rect(w / 2 - hw, h / 2 - hh, hw * 2, hh * 2) } else { ctx.arc(w / 2, h / 2, half(fv.w, fr.u), 0, Math.PI * 2) }
      ctx.stroke()
      ctx.fillStyle = 'rgba(255, 150, 150, 0.95)'; ctx.font = '600 11px system-ui, sans-serif'; ctx.textAlign = 'center'
      ctx.fillText(fv.label, w / 2, h / 2 + (fv.h ? half(fv.h, fr.u) : half(fv.w, fr.u)) + 14)
      ctx.restore()
    }
    if (st.mode === 'point') {
      const g = p.guide || null
      setGuide(prev => (!g && !prev) || (g && prev && prev.key === g.key && prev.onScreen === g.onScreen && prev.turn === g.turn) ? prev : g)
    }
    if (st.mode === 'point') {
      const [alt, az] = altAz(fr.f)
      const hit = hitTest({ ...p, hits: p.hits }, w / 2, h / 2, null)
      const close = hit && Math.hypot(hit.x - w / 2, hit.y - h / 2) < 46 ? hit : null
      setAim(prev => {
        const next = { alt: Math.round(alt), az: Math.round(az) % 360, name: close?.name || '', key: close?.key || '' }
        return prev && prev.alt === next.alt && prev.az === next.az && prev.key === next.key ? prev : next
      })
    }
  }, [cat])

  const schedule = useCallback(() => { if (!raf.current) raf.current = requestAnimationFrame(paint) }, [paint])
  useEffect(() => { schedule() }, [schedule, t, obs, layers, mode, camOn, selected, cat, settings?.mag_limit, camFov, path, guideKey, fov])

  // canvas size
  useEffect(() => {
    const el = wrap.current, c = canvas.current
    if (!el || !c) return
    const fit = () => {
      const r = el.getBoundingClientRect()
      const dpr = Math.min(window.devicePixelRatio || 1, 2.5)
      size.current = { w: r.width, h: r.height, dpr }
      c.width = Math.round(r.width * dpr); c.height = Math.round(r.height * dpr)
      c.style.width = r.width + 'px'; c.style.height = r.height + 'px'
      schedule()
    }
    fit()
    const ro = new ResizeObserver(fit)
    ro.observe(el)
    return () => ro.disconnect()
  }, [schedule, mode])

  const keepView = () => localStorage.setItem(VIEW_KEY, JSON.stringify(view.current))

  const pq = placeQuery(place)
  // comets and asteroids (they move slowly: once per place and day)
  const day = Math.floor(t / 86400000)
  useEffect(() => {
    if (!pq) return
    api.get<{ objects: MinorObj[] }>(`${V1}/minor?${pq}&t=${day * 86400000 + 43200000}`).then(r => {
      minor.current = r.objects
      for (const m of r.objects) registerExtra(m.key, m.ra, m.dec, true)
      schedule()
    }).catch(() => {})
  }, [pq, day, schedule])

  // satellites above the horizon, fetched every 20 seconds while the time is now
  useEffect(() => {
    if (!pq || !live) { sats.current = null; return }
    let alive = true
    const get = () => api.get<{ satellites: SatNow[] }>(`${V1}/satellites/now?${pq}`).then(r => { if (alive) { sats.current = { at: Date.now(), list: r.satellites }; schedule() } }).catch(() => {})
    get()
    const id = window.setInterval(() => { if (document.visibilityState === 'visible') get() }, 20000)
    return () => { alive = false; window.clearInterval(id) }
  }, [pq, live, schedule])

  // ---- going to an object ----
  const goTo = useCallback((key: string, glide = true) => {
    const st = stateRef.current
    if (!st.obs) return
    const d = directionOf(cat, key, st.obs, st.t)
    if (!d) return
    if (stateRef.current.mode === 'point') return
    const to = { az: d.az, alt: Math.max(-20, Math.min(85, d.alt)) }
    if (glide) anim.current = { from: { az: view.current.az, alt: view.current.alt }, to, t0: performance.now() }
    else { view.current.az = to.az; view.current.alt = to.alt }
    if (view.current.fov > 100) view.current.fov = 80
    keepView(); schedule()
  }, [cat, schedule])

  // ?plan=ID tours that observing plan
  useEffect(() => {
    const pid = sp.get('plan')
    if (!pid || !cat || !obs) return
    api.get<{ items: { key: string; name: string; words?: string; note: string }[] }>(`${V1}/plans/${pid}`).then(pl => {
      const steps: TourStep[] = pl.items.map(it => ({ key: it.key, name: it.name, text: [it.words, it.note].filter(Boolean).join(' '), az: 0 }))
      if (steps.length) showStep(steps, 0)
    }).catch(() => {})
    const next = new URLSearchParams(sp); next.delete('plan'); setSp(next, { replace: true })
  }, [sp, cat, obs]) // eslint-disable-line react-hooks/exhaustive-deps

  // ?object=dso:M31 opens it
  useEffect(() => {
    const k = sp.get('object')
    if (!k || !cat || !obs) return
    setSelected(k); goTo(k, false)
    const next = new URLSearchParams(sp); next.delete('object'); setSp(next, { replace: true })
  }, [sp, cat, obs, goTo, setSp])

  // ---- gestures ----
  const pts = useRef(new Map<number, { x: number; y: number }>())
  const gesture = useRef<{ x: number; y: number; t: number; moved: boolean; dist?: number; fov?: number; az: number; alt: number; off: number } | null>(null)
  const onDown = (e: React.PointerEvent) => {
    (e.target as HTMLElement).setPointerCapture(e.pointerId)
    pts.current.set(e.pointerId, { x: e.clientX, y: e.clientY })
    anim.current = null
    const ps = [...pts.current.values()]
    const g = { x: e.clientX, y: e.clientY, t: performance.now(), moved: false, az: view.current.az, alt: view.current.alt, off: pointer.current?.offset || 0 } as NonNullable<typeof gesture.current>
    if (ps.length === 2) { g.dist = Math.hypot(ps[0].x - ps[1].x, ps[0].y - ps[1].y); g.fov = stateRef.current.mode === 'point' && stateRef.current.camOn ? stateRef.current.camFov : view.current.fov; g.moved = true }
    gesture.current = g
  }
  const camFovLive = useRef<number | null>(null)
  const onMove = (e: React.PointerEvent) => {
    if (!pts.current.has(e.pointerId)) return
    pts.current.set(e.pointerId, { x: e.clientX, y: e.clientY })
    const g = gesture.current
    if (!g) return
    const ps = [...pts.current.values()]
    const { h } = size.current
    if (ps.length === 2 && g.dist && g.fov) {
      const d = Math.hypot(ps[0].x - ps[1].x, ps[0].y - ps[1].y)
      const nf = Math.max(8, Math.min(140, g.fov * g.dist / Math.max(20, d)))
      if (stateRef.current.mode === 'point' && stateRef.current.camOn) { camFovLive.current = Math.max(20, Math.min(110, nf)); stateRef.current.camFov = camFovLive.current }
      else view.current.fov = nf
      schedule(); return
    }
    const dx = e.clientX - g.x, dy = e.clientY - g.y
    if (Math.hypot(dx, dy) > 6) g.moved = true
    if (!g.moved) return
    const degPx = view.current.fov / Math.max(200, h)
    if (stateRef.current.mode === 'point') {
      if (pointer.current) pointer.current.offset = g.off - dx * degPx * 0.8
    } else {
      view.current.az = (g.az - dx * degPx * 1.1 + 720) % 360
      view.current.alt = Math.max(-89, Math.min(89.9, g.alt + dy * degPx * 1.1))
    }
    schedule()
  }
  const onUp = (e: React.PointerEvent) => {
    const g = gesture.current
    pts.current.delete(e.pointerId)
    if (pts.current.size > 0) { gesture.current = { ...g!, x: [...pts.current.values()][0].x, y: [...pts.current.values()][0].y, dist: undefined, az: view.current.az, alt: view.current.alt, off: pointer.current?.offset || 0 }; return }
    gesture.current = null
    if (camFovLive.current != null) { saveSetting({ camera_fov: Math.round(camFovLive.current) }); camFovLive.current = null }
    keepView()
    if (g && !g.moved && performance.now() - g.t < 500 && painted.current) {
      const r = canvas.current!.getBoundingClientRect()
      const hit = hitTest(painted.current, e.clientX - r.left, e.clientY - r.top, cat)
      if (hit?.kind === 'minor') { setMinorSel(minor.current.find(x => x.key === hit.key) || null); setSelected(''); setSatSel(null) }
      else if (hit?.kind === 'sat') { setSatSel(sats.current?.list.find(x => 'sat:' + x.norad === hit.key) || null); setSelected('') }
      else if (hit) { setSelected(hit.key); setSelName(hit.name); setSatSel(null) } else { setSelected(''); setSatSel(null) }
    }
  }
  const onWheel = (e: React.WheelEvent) => {
    view.current.fov = Math.max(8, Math.min(140, view.current.fov * Math.exp(e.deltaY * 0.0012)))
    keepView(); schedule()
  }
  useEffect(() => {
    const el = canvas.current
    if (!el) return
    const prevent = (e: WheelEvent) => e.preventDefault()
    el.addEventListener('wheel', prevent, { passive: false })
    return () => el.removeEventListener('wheel', prevent)
  }, [mode])
  useEffect(() => {
    const k = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement).closest('input, textarea, [contenteditable="true"]')) return
      const step = view.current.fov / 12
      let used = true
      switch (e.key) {
        case 'ArrowLeft': view.current.az = (view.current.az - step + 360) % 360; break
        case 'ArrowRight': view.current.az = (view.current.az + step) % 360; break
        case 'ArrowUp': view.current.alt = Math.min(89.9, view.current.alt + step); break
        case 'ArrowDown': view.current.alt = Math.max(-89, view.current.alt - step); break
        case '+': case '=': view.current.fov = Math.max(8, view.current.fov / 1.2); break
        case '-': view.current.fov = Math.min(140, view.current.fov * 1.2); break
        case 'Escape': setSelected(''); break
        default: used = false
      }
      if (used) { e.preventDefault(); keepView(); schedule() }
    }
    window.addEventListener('keydown', k)
    return () => window.removeEventListener('keydown', k)
  }, [schedule])

  // ---- pointing the phone ----
  // keep the screen on while pointing at the sky
  const wake = useRef<{ release: () => Promise<void> } | null>(null)
  const wakeOn = () => {
    const nav = navigator as Navigator & { wakeLock?: { request: (t: 'screen') => Promise<{ release: () => Promise<void> }> } }
    nav.wakeLock?.request('screen').then(l => { wake.current = l }).catch(() => {})
  }
  useEffect(() => {
    const h = () => { if (document.visibilityState === 'visible' && pointer.current) wakeOn() }
    document.addEventListener('visibilitychange', h)
    return () => document.removeEventListener('visibilitychange', h)
  }, [])
  const stopPointing = useCallback(() => {
    wake.current?.release().catch(() => {}); wake.current = null
    if (pointer.current) sessionStorage.setItem('psky-offset', String(pointer.current.offset))
    pointer.current?.stop(); pointer.current = null; frame.current = null
    stream.current?.getTracks().forEach(tr => tr.stop()); stream.current = null
    setCamOn(false); setMode('map'); setNoSensor(false)
  }, [])
  useEffect(() => () => { pointer.current?.stop(); stream.current?.getTracks().forEach(tr => tr.stop()) }, [])
  const startCamera = async () => {
    try {
      const s = await openCamera()
      stream.current = s
      setCamOn(true)
      setCamErr('')
      requestAnimationFrame(() => { if (video.current) { video.current.srcObject = s; video.current.play().catch(() => {}) } })
    } catch (e) {
      const msg = (e as Error).name === 'NotAllowedError' ? 'The camera is not allowed for this site (Settings → Safari → Camera, or the aA menu → Website Settings). The stars work without it.' : (e as Error).message
      setCamErr(msg)
    }
  }
  const stopCamera = () => { stream.current?.getTracks().forEach(tr => tr.stop()); stream.current = null; setCamOn(false) }
  const startPointing = async (withCamera: boolean) => {
    if (!sensorsAvailable()) { toast('This device has no compass or tilt sensor. Drag the map instead.', 'err'); return }
    const ok = await askSensors()
    if (!ok) { toast('Motion and orientation are not allowed for this site.', 'err'); return }
    setSelected('')
    const p = new Pointer(fr => { frame.current = fr; setNorthFound(p.northFound); schedule() })
    p.offset = Number(sessionStorage.getItem('psky-offset') || 0) || 0
    pointer.current = p
    p.start()
    wakeOn()
    setMode('point')
    setNoSensor(false)
    window.setTimeout(() => { if (pointer.current === p && !p.heard) setNoSensor(true) }, 2500)
    if (withCamera) await startCamera()
  }

  // ---- sharing a picture of the sky ----
  const share = async () => {
    const c = canvas.current
    if (!c) return
    const out = document.createElement('canvas')
    const band = Math.round(46 * size.current.dpr)
    out.width = c.width; out.height = c.height + band
    const x = out.getContext('2d')!
    x.drawImage(c, 0, 0)
    x.fillStyle = '#0b1020'; x.fillRect(0, c.height, out.width, band)
    x.fillStyle = '#e6ebff'; x.font = `600 ${Math.round(15 * size.current.dpr)}px system-ui, sans-serif`; x.textBaseline = 'middle'
    const when = `${fmtDayAt(t, place?.timezone, true)} ${fmtAt(t, place?.timezone)}`
    x.fillText(`The sky over ${place?.name || 'here'} · ${when}`, 14 * size.current.dpr, c.height + band / 2)
    x.textAlign = 'right'; x.fillStyle = '#9aa5cf'
    x.fillText('Pocket Sky', out.width - 14 * size.current.dpr, c.height + band / 2)
    const blob = await new Promise<Blob | null>(res => out.toBlob(res, 'image/png'))
    if (!blob) return
    const file = new File([blob], `sky-${new Date(t).toISOString().slice(0, 16).replace(':', '')}.png`, { type: 'image/png' })
    const nav = navigator as Navigator & { canShare?: (d: { files: File[] }) => boolean }
    if (nav.canShare?.({ files: [file] })) {
      try { await navigator.share({ files: [file], title: 'The sky', text: `The sky over ${place?.name || 'here'}, ${when}` }); return } catch { /* cancelled */ return }
    }
    const a = document.createElement('a')
    a.href = URL.createObjectURL(file); a.download = file.name; a.click()
    window.setTimeout(() => URL.revokeObjectURL(a.href), 4000)
    toast('Picture saved', 'ok')
  }

  // ---- tonight's tour ----
  const showStep = (steps: TourStep[], i: number) => {
    const st = steps[i]
    setTour({ steps, i })
    setSelected(st.key); setSelName(st.name)
    if (stateRef.current.mode === 'point') { setGuideKey(st.key); setGuideName(st.name) } else goTo(st.key)
  }
  const startTour = async () => {
    if (!place || !obs) return
    try {
      const v = await api.get<Tonight>(`${V1}/tonight?${placeQuery(place)}`)
      const steps = buildTour(v, cat, obs, Date.now())
      if (steps.length === 0) { toast('Nothing good is up right now; try after dark.'); return }
      showStep(steps, 0)
    } catch (e) { toast((e as Error).message, 'err') }
  }

  // ---- settings from the map ----
  const saveSetting = useCallback((patch: Partial<Settings>) => {
    if (settings) setSettings({ ...settings, ...patch } as Settings)
    api.put<Settings>(`${V1}/settings`, patch).then(s => setSettings(s)).catch(() => {})
  }, [settings, setSettings])
  const layerItems = (): MenuItem[] => [
    { heading: 'Show on the map' },
    { label: 'Constellation lines', checked: layers.lines, onSelect: () => saveSetting({ show_lines: !layers.lines }) },
    { label: 'Constellation names', checked: layers.conNames, onSelect: () => saveSetting({ show_con_names: !layers.conNames }) },
    { label: 'Star names', checked: layers.starNames, onSelect: () => saveSetting({ show_star_names: !layers.starNames }) },
    { label: 'Clusters, nebulae, galaxies', checked: layers.dso, onSelect: () => saveSetting({ show_dso: !layers.dso }) },
    { label: 'Milky Way', checked: layers.milkyWay, onSelect: () => saveSetting({ show_milky_way: !layers.milkyWay }) },
    { label: "The Sun's path", checked: layers.ecliptic, onSelect: () => saveSetting({ show_ecliptic: !layers.ecliptic }) },
    { label: 'Height and direction grid', checked: layers.grid, onSelect: () => saveSetting({ show_grid: !layers.grid }) },
    { label: 'Sky equator grid', checked: layers.equator, onSelect: () => saveSetting({ show_equator: !layers.equator }) },
    'sep',
    { heading: 'Stars' },
    ...[4, 5, 5.5, 6, 6.5].map(m => ({ label: m <= 4 ? 'Bright stars only' : m === 6.5 ? 'All the stars (dark sky)' : `Down to magnitude ${m}`, checked: Math.abs((settings?.mag_limit ?? 5.5) - m) < 0.01, onSelect: () => saveSetting({ mag_limit: m }) })),
    'sep',
    { heading: 'Field of view' },
    { label: 'None', checked: !fov, onSelect: () => { setFov(null); saveFov(null) } },
    ...setups(kit).map(su => ({ label: su.label, checked: fov?.id === su.id, onSelect: () => { setFov(su); saveFov(su) } })),
    ...(kit.length === 0 ? [{ label: 'Add your equipment…', onSelect: () => navigate('/plans') }] : []),
  ]

  // ---- search ----
  const [q, setQ] = useState('')
  const dq = useDebounce(q.trim(), 200)
  const [results, setResults] = useState<SkyObject[] | null>(null)
  const [searchOpen, setSearchOpen] = useState(false)
  useEffect(() => {
    if (dq.length < 1) { setResults(null); return }
    let alive = true
    api.get<SkyObject[]>(`${V1}/objects/search?q=${encodeURIComponent(dq)}&limit=12`).then(r => { if (alive) setResults(r) }).catch(() => {})
    return () => { alive = false }
  }, [dq])
  const pick = (o: SkyObject) => {
    setSearchOpen(false); setQ('')
    if (stateRef.current.mode === 'point') { setGuideKey(o.key); setGuideName(o.name); setSelected(''); return }
    setSelected(o.key); setSelName(o.name); goTo(o.key)
  }

  const timeLabel = live ? `Now · ${fmtAt(t, place?.timezone)}` : `${fmtDayAt(t, place?.timezone)} ${fmtAt(t, place?.timezone)}`
  const setTime = (ms: number) => { setLive(false); setT(ms) }

  if (loaded && !place) {
    return (
      <div className="sky-empty">
        <Icon name="map" size={28} />
        <h2>Where do you watch the sky from?</h2>
        <p className="muted">The map needs a place: add one, or use where you are now.</p>
        <div className="row" style={{ gap: 8, justifyContent: 'center' }}>
          <button type="button" className="btn primary" onClick={() => locate()}><Icon name="locate" size={15} />Use where I am</button>
          <PlacePicker />
        </div>
      </div>
    )
  }

  const pointing = mode === 'point'
  return (
    <div className={`sky-page ${pointing ? 'sky-point' : ''} ${night ? 'night-red' : ''}`}>
      <div className="sky-stage" ref={wrap}>
        {pointing && camOn && <video ref={video} className="sky-video" playsInline muted autoPlay />}
        <canvas ref={canvas} className="sky-canvas" onPointerDown={onDown} onPointerMove={onMove} onPointerUp={onUp} onPointerCancel={onUp} onWheel={onWheel}
          aria-label="Star map. Drag to look around, pinch or scroll to zoom, tap a star or planet to see what it is." role="img" />
        {!cat && !catErr && <div className="sky-loading"><Spinner /> Loading the stars…</div>}
        {catErr && <div className="sky-loading bad">{catErr}</div>}

        {/* top bar */}
        <div className="sky-bar top">
          {pointing && <button type="button" className="sky-btn icon" onClick={stopPointing} aria-label="Back to the map" title="Back to the map"><Icon name="x" size={18} /></button>}
          {(
            <div className={`sky-search ${pointing ? 'compact' : ''}`}>
              <Icon name="search" size={16} />
              <input value={q} onChange={e => { setQ(e.target.value); setSearchOpen(true) }} onFocus={() => setSearchOpen(true)} placeholder={pointing ? 'Find…' : 'Find a star, planet, galaxy…'} aria-label="Find in the sky" />
              {searchOpen && results && (
                <div className="sky-results">
                  {results.length === 0 && <div className="muted small" style={{ padding: 10 }}>Nothing called “{dq}”.</div>}
                  {results.map(o => (
                    <button key={o.key} type="button" className="sky-result" onMouseDown={e => e.preventDefault()} onClick={() => pick(o)}>
                      <span className="grow ellipsis">{o.name}</span><span className="muted small">{o.type}</span>
                    </button>
                  ))}
                </div>
              )}
            </div>
          )}
          <span className="spacer" />
          {!pointing && <PlacePicker className="sky-pick hide-sm" />}
          <MenuButton label="What to show" className="sky-btn icon" align="right" width={260} items={layerItems}><Icon name="layers" size={18} /></MenuButton>
          <button type="button" className={`sky-btn icon ${night ? 'on' : ''}`} onClick={() => saveSetting({ night_red: !night })} aria-pressed={night} title="Red light, to keep your eyes used to the dark"><Icon name="eye" size={18} /></button>
          {pointing && <button type="button" className={`sky-btn ${camOn ? 'on' : ''}`} onClick={() => (camOn ? stopCamera() : startCamera())} aria-pressed={camOn}><Icon name="camera" size={18} /><span className="hide-sm">{camOn ? 'Camera on' : 'Camera off'}</span></button>}
        </div>

        {/* point mode readout */}
        {pointing && (
          <div className="sky-aim">
            <Icon name="crosshair" size={44} strokeWidth={1.2} className="sky-cross" />
            {aim?.name && <button type="button" className="sky-btn sky-aim-name" onClick={() => { setSelected(aim.key); setSelName(aim.name) }}><b>{aim.name}</b></button>}
            {aim && <span className="sky-aim-dir">{aim.alt}° up · {dirShort(aim.az)} {aim.az}°</span>}
            {!northFound && !noSensor && <div className="sky-hint">Finding north: hold the phone upright for a moment, camera towards the horizon.</div>}
            {camErr && northFound && !noSensor && <div className="sky-hint">{camErr}</div>}
            {noSensor && <div className="sky-hint">No compass readings yet. Move the phone in a figure of eight; if nothing happens, this device has no compass.</div>}
            {!guideKey && !tour && <div className="sky-hint soft">Off a little? Tap Line up, or drag sideways.{camOn ? ' Pinch to match the camera.' : ''}</div>}
          </div>
        )}
        {pointing && guideKey && (
          <div className={`sky-guide ${guide?.onScreen ? 'found' : ''}`}>
            <Icon name={guide?.onScreen ? 'check-circle' : 'navigation'} size={16} />
            <span className="grow"><b>{guideName}</b>{guide ? (guide.onScreen ? (guide.off < 4 ? ': in the crosshair' : ': in view, bring it to the centre') : `: ${guide.turn || 'nearly there'}`) : ''}</span>
            <button type="button" className="sky-btn icon sm" onClick={() => { setGuideKey(''); setGuide(null) }} aria-label="Stop looking for it"><Icon name="x" size={14} /></button>
          </div>
        )}
        {pointing && lineUp && obs && <LineUp obs={obs} t={t} cat={cat} onClose={() => setLineUp(false)} onPick={(name, az) => {
          const f = frame.current
          if (!f || !pointer.current) return
          const [, aimAz] = altAz(f.f)
          const d = ((az - aimAz + 540) % 360) - 180
          pointer.current.offset += d
          sessionStorage.setItem('psky-offset', String(pointer.current.offset))
          setLineUp(false)
          toast(`Lined up on ${name}${Math.abs(d) >= 1 ? ` (moved ${Math.abs(Math.round(d))}°)` : ''}`, 'ok')
        }} />}

        {/* bottom bar */}
        <div className="sky-bar bottom">
          <button type="button" className={`sky-btn sky-when ${timeOpen ? 'on' : ''}`} onClick={() => { setBase(live ? Date.now() : t); setTimeOpen(o => !o) }} aria-expanded={timeOpen} title={timeLabel}><Icon name="clock" size={16} /><span className="ellipsis">{live ? <><span className="hide-sm">Now · </span>{fmtAt(t, place?.timezone)}</> : timeLabel}</span></button>
          {!live && <button type="button" className="sky-btn" onClick={() => { setLive(true); setT(Date.now()) }}>Now</button>}
          <span className="spacer" />
          {pointing && <button type="button" className={`sky-btn ${lineUp ? 'on' : ''}`} onClick={() => setLineUp(o => !o)} title="Put the crosshair on the Moon or a bright star, then tap its name"><Icon name="crosshair" size={16} />Line up</button>}
          {!pointing && <button type="button" className="sky-btn icon" onClick={share} aria-label="Share a picture of this sky" title="Share a picture of this sky (set any date in the time panel: the sky on a birthday)"><Icon name="share" size={16} /></button>}
          <button type="button" className={`sky-btn ${tour ? 'on' : ''}`} onClick={() => (tour ? setTour(null) : startTour())} title="Tonight's best sights, one after the other"><Icon name="route" size={16} /><span className={pointing ? '' : 'hide-sm'}>Tour</span></button>
          {!pointing && <>
            <button type="button" className="sky-btn icon hide-sm" onClick={() => { view.current.fov = Math.max(8, view.current.fov / 1.4); keepView(); schedule() }} aria-label="Zoom in"><Icon name="plus" size={18} /></button>
            <button type="button" className="sky-btn icon hide-sm" onClick={() => { view.current.fov = Math.min(140, view.current.fov * 1.4); keepView(); schedule() }} aria-label="Zoom out"><Icon name="minus" size={18} /></button>
            <button type="button" className="sky-btn primary" onClick={() => startPointing(true)} title="Hold your phone up to the sky"><Icon name="compass" size={16} />Point<span className="hide-sm">&nbsp;at the sky</span></button>
          </>}
        </div>
        {timeOpen && (
          <div className="sky-time">
            <div className="row" style={{ gap: 6, flexWrap: 'wrap' }}>
              <button type="button" className="btn sm" onClick={() => setTime(t - 86400000)}><Icon name="chevron-left" size={14} />Day</button>
              <button type="button" className="btn sm" onClick={() => setTime(t - 3600000)}>−1 h</button>
              <input type="date" className="input sm" value={new Date(t).toISOString().slice(0, 10)} onChange={e => { const d = new Date(e.target.value + 'T' + new Date(t).toISOString().slice(11)); if (!Number.isNaN(d.getTime())) { setBase(d.getTime()); setTime(d.getTime()) } }} aria-label="Date" />
              <button type="button" className="btn sm" onClick={() => setTime(t + 3600000)}>+1 h</button>
              <button type="button" className="btn sm" onClick={() => setTime(t + 86400000)}>Day<Icon name="chevron-right" size={14} /></button>
              <span className="spacer" />
              <button type="button" className="btn sm ghost" onClick={() => { setLive(true); setT(Date.now()); setTimeOpen(false) }}>Back to now</button>
            </div>
            <input type="range" className="sky-slider" min={-12 * 60} max={12 * 60} step={5} value={Math.round((t - base) / 60000)} onChange={e => setTime(base + Number(e.target.value) * 60000)} aria-label="Move the time" />
            <div className="row muted small" style={{ justifyContent: 'space-between' }}><span>−12 h</span><span>{fmtDayAt(t, place?.timezone)} {fmtAt(t, place?.timezone)}</span><span>+12 h</span></div>
          </div>
        )}
        {selected && obs && !tour && <InfoSheet k={selected} name={selName} t={t} placeQ={placeQuery(place)} cat={cat} obs={obs} tz={place?.timezone}
          onClose={() => setSelected('')} onCenter={() => goTo(selected)} pointing={pointing} onLog={() => navigate(`/log?add=${encodeURIComponent(selected)}`)}
          path={path === selected} onPath={() => setPath(p => (p === selected ? '' : selected))}
          onFind={() => { setGuideKey(selected); setGuideName(selName || selected); setSelected('') }} />}
        {satSel && !tour && !selected && <SatSheet s={satSel} onClose={() => setSatSel(null)} />}
        {minorSel && !tour && !selected && <MinorSheet m={minorSel} tz={place?.timezone} onClose={() => setMinorSel(null)} onPath={() => { setPath(minorSel.key) }} />}
        {tour && <TourPanel tour={tour} onStep={i => showStep(tour.steps, i)} onEnd={() => { setTour(null); setGuideKey(''); setSelected('') }} />}
      </div>
    </div>
  )
}

// ---- what you tapped ----

function InfoSheet({ k, name, t, placeQ, cat, obs, tz, onClose, onCenter, pointing, onLog, path, onPath, onFind }: {
  k: string; name: string; t: number; placeQ: string; cat: Catalog | null; obs: { lat: number; lon: number }; tz?: string
  onClose: () => void; onCenter: () => void; pointing: boolean; onLog: () => void; path: boolean; onPath: () => void; onFind: () => void
}) {
  const { toast } = useToast()
  const [d, setD] = useState<ObjectDetail | null>(null)
  const [err, setErr] = useState('')
  const [ai, setAi] = useState<{ busy: boolean; text: string }>({ busy: false, text: '' })
  const tq = Math.floor(t / 60000) // refetch at most a minute apart
  useEffect(() => {
    let alive = true
    setErr('')
    api.get<ObjectDetail>(`${V1}/objects/${encodeURIComponent(k)}?${placeQ}&t=${tq * 60000}`).then(r => { if (alive) setD(r) }).catch(e => { if (alive) setErr((e as Error).message) })
    return () => { alive = false }
  }, [k, placeQ, tq])
  useEffect(() => { setAi({ busy: false, text: '' }); setD(null) }, [k])
  // the position now, from the browser engine (follows the time slider live)
  const live = useMemo(() => {
    if ((BODIES as string[]).includes(k)) { const s = at(k as BodyKey, obs, t); return { alt: s.alt, az: s.az, mag: s.mag, phase: s.phase } }
    const dd = directionOf(cat, k, obs, t)
    return dd ? { ...dd, mag: undefined as number | undefined, phase: undefined as number | undefined } : null
  }, [k, obs, t, cat])
  const star = k.startsWith('star:') && cat ? cat.stars[cat.named.get(Number(k.slice(5))) ?? -1] : undefined
  const title = d?.name || (star ? starLabel(cat!, star) : '') || name || (NAMES as Record<string, string>)[k] || k
  const toggleTarget = async () => {
    if (!d) return
    try {
      if (d.target_id) { await api.del(`${V1}/targets/${d.target_id}`); setD({ ...d, target_id: 0 }); toast('Taken off your list') }
      else { const r = await api.post<{ id: number }>(`${V1}/targets`, { key: k }); setD({ ...d, target_id: r.id }); toast('On your want-to-see list', 'ok') }
    } catch (e) { toast((e as Error).message, 'err') }
  }
  const explain = async () => {
    setAi({ busy: true, text: '' })
    try {
      const r = await api.post<{ ok: boolean; text?: string; error?: string }>(`${V1}/ai/explain`, { key: k })
      setAi({ busy: false, text: r.ok ? r.text || '' : r.error || 'Pocket AI did not answer' })
    } catch (e) { setAi({ busy: false, text: (e as Error).message }) }
  }
  const up = live ? live.alt > 0 : d?.up
  return (
    <div className="sky-sheet" role="dialog" aria-label={title}>
      <div className="sky-sheet-head">
        <div className="grow">
          <div className="sky-sheet-title">{title}</div>
          <div className="muted small">{[d?.type, d?.con_name ? `in ${d.con_name}` : '', d?.alt_name].filter(Boolean).join(' · ')}</div>
        </div>
        <button type="button" className="btn ghost icon-only" onClick={onClose} aria-label="Close"><Icon name="x" size={18} /></button>
      </div>
      {err && <p className="bad small">{err}</p>}
      {live && <div className="sky-sheet-now">
        <span className={`pill ${up ? 'ok' : ''}`}>{up ? `${Math.round(live.alt)}° up` : 'Below the horizon'}</span>
        <span>{up ? `in the ${direction(live.az)}` : ''}</span>
        {d?.mag != null && <span className="muted">magnitude {d.mag.toFixed(1)}</span>}
        {live.phase != null && k !== 'sun' && <span className="muted">{Math.round(live.phase * 100)}% lit</span>}
      </div>}
      {d && <>
        <dl className="sky-facts">
          {d.always === 'up' ? <><dt>Today</dt><dd>Never sets here</dd></> : d.always === 'down' ? <><dt>Today</dt><dd>Never rises here</dd></> : <>
            {d.rise && <><dt>Rises</dt><dd>{fmtAt(d.rise, tz)}</dd></>}
            {d.transit && <><dt>Highest</dt><dd>{fmtAt(d.transit, tz)}</dd></>}
            {d.set && <><dt>Sets</dt><dd>{fmtAt(d.set, tz)}</dd></>}
          </>}
          {d.body && d.body.dist_km > 0 && <><dt>Distance</dt><dd>{d.body.dist_au > 0.05 ? `${d.body.dist_au.toFixed(2)} AU (${Math.round(d.body.dist_km / 1e6).toLocaleString()} million km)` : `${Math.round(d.body.dist_km).toLocaleString()} km`}</dd></>}
        </dl>
        {d.tonight && <p className="small">{d.tonight}</p>}
        {d.about && <p className="small muted">{d.about}</p>}
        {d.seen && <p className="small"><Icon name="check-circle" size={14} className="ok" /> You logged it {d.seen.times === 1 ? 'once' : `${d.seen.times} times`}, first on {fmtDate(d.seen.first)}.</p>}
        {(k === 'jupiter' || k === 'saturn') && <PlanetDetail k={k} t={t} tz={tz} />}
        {ai.text && <p className="small sky-ai">{ai.text}</p>}
      </>}
      <div className="row" style={{ gap: 6, flexWrap: 'wrap' }}>
        {!pointing && <button type="button" className="btn sm" onClick={onCenter}><Icon name="crosshair" size={14} />Centre</button>}
        {pointing && <button type="button" className="btn sm" onClick={onFind}><Icon name="navigation" size={14} />Find it</button>}
        {k !== 'sun' && <button type="button" className={`btn sm ${path ? 'on' : ''}`} onClick={onPath} aria-pressed={path}><Icon name="route" size={14} />{path ? 'Hide its path' : 'Path tonight'}</button>}
        {d && <button type="button" className="btn sm" onClick={toggleTarget}><Icon name={d.target_id ? 'check' : 'target'} size={14} />{d.target_id ? 'On your list' : 'Want to see'}</button>}
        <button type="button" className="btn sm" onClick={onLog}><Icon name="journal" size={14} />Log it</button>
        {k === 'moon' && <MoonMapButton t={t} />}
        <button type="button" className="btn sm" onClick={explain} disabled={ai.busy}>{ai.busy ? <Spinner /> : <Icon name="sparkles" size={14} />}Tell me more</button>
      </div>
    </div>
  )
}

// ---- the tour ----

interface TourStep { key: string; name: string; text: string; az: number }

/** Tonight's best things that are up now, in a sweep round the sky from the north through the east. */
function buildTour(v: Tonight, cat: Catalog | null, obs: { lat: number; lon: number }, t: number): TourStep[] {
  const out: TourStep[] = []
  const add = (key: string, name: string, text: string, minAlt: number) => {
    const d = directionOf(cat, key, obs, t)
    if (d && d.alt > minAlt && !out.some(s => s.key === key)) out.push({ key, name, text, az: d.az })
  }
  if (v.moon.phase > 0.03) add('moon', 'Moon', v.moon.words, 3)
  for (const p of v.planets) if (p.visible) add(p.key, p.name, p.words, 5)
  for (const x of v.sights.slice(0, 7)) add(x.key, x.name, x.words, 15)
  if (cat) {
    const cons = cat.cons.filter(c => c.rank === 1).map(c => ({ c, d: directionOf(cat, 'con:' + c.id, obs, t) })).filter(x => x.d && x.d.alt > 30)
      .sort((a, b) => b.d!.alt - a.d!.alt).slice(0, 3)
    for (const { c } of cons) add('con:' + c.id, c.name, c.about, 30)
  }
  return out.sort((a, b) => a.az - b.az)
}

function TourPanel({ tour, onStep, onEnd }: { tour: { steps: TourStep[]; i: number }; onStep: (i: number) => void; onEnd: () => void }) {
  const st = tour.steps[tour.i]
  return (
    <div className="sky-sheet sky-tour" role="dialog" aria-label="Tonight's tour">
      <div className="sky-sheet-head">
        <div className="grow"><div className="muted small">Tonight's tour · {tour.i + 1} of {tour.steps.length}</div><div className="sky-sheet-title">{st.name}</div></div>
        <button type="button" className="btn ghost icon-only" onClick={onEnd} aria-label="End the tour"><Icon name="x" size={18} /></button>
      </div>
      <p className="small">{st.text}</p>
      <div className="row" style={{ gap: 6 }}>
        <button type="button" className="btn sm" disabled={tour.i === 0} onClick={() => onStep(tour.i - 1)}><Icon name="chevron-left" size={14} />Back</button>
        <span className="spacer" />
        {tour.i < tour.steps.length - 1 ? <button type="button" className="btn sm primary" onClick={() => onStep(tour.i + 1)}>Next<Icon name="chevron-right" size={14} /></button>
          : <button type="button" className="btn sm primary" onClick={onEnd}>Done</button>}
      </div>
    </div>
  )
}

// ---- lining the compass up on something you can see ----

function LineUp({ obs, t, cat, onPick, onClose }: { obs: { lat: number; lon: number }; t: number; cat: Catalog | null; onPick: (name: string, az: number) => void; onClose: () => void }) {
  const list = useMemo(() => {
    const out: { name: string; az: number; alt: number; mag: number }[] = []
    for (const b of BODIES) {
      if (b === 'sun') continue
      const s = at(b, obs, t)
      if (s.alt > 3 && s.mag < 1.6) out.push({ name: NAMES[b], az: s.az, alt: s.alt, mag: s.mag })
    }
    if (cat) {
      for (const st of cat.stars) {
        if (st[2] > 1.3) break
        if (!st[5]) continue
        const d = directionOf(cat, 'star:' + st[4], obs, t)
        if (d && d.alt > 5) out.push({ name: st[5], az: d.az, alt: d.alt, mag: st[2] })
      }
    }
    return out.sort((a, b) => a.mag - b.mag).slice(0, 8)
  }, [obs, t, cat])
  return (
    <div className="sky-sheet sky-lineup" role="dialog" aria-label="Line up">
      <div className="sky-sheet-head">
        <div className="grow"><div className="sky-sheet-title">Line up</div><div className="muted small">Put the crosshair on one of these in the real sky, then tap its name.</div></div>
        <button type="button" className="btn ghost icon-only" onClick={onClose} aria-label="Close"><Icon name="x" size={18} /></button>
      </div>
      {list.length === 0 && <p className="small muted">Nothing bright enough is up now.</p>}
      <div className="lineup-list">
        {list.map(x => <button key={x.name} type="button" className="btn sm" onClick={() => onPick(x.name, x.az)}>{x.name}<span className="muted small">{Math.round(x.alt)}° up {dirShort(x.az)}</span></button>)}
      </div>
    </div>
  )
}

// ---- satellites ----

/** Satellites moved to a moment: straight-line between now and a minute on (fine for a minute or two). */
function satsAt(cur: { at: number; list: SatNow[] }, t: number) {
  const k = Math.max(0, Math.min(2.5, (t - cur.at) / 60000))
  return cur.list.map(s => {
    let daz = ((s.az2 - s.az + 540) % 360) - 180
    if (Math.abs(daz) > 90) daz = 0 // near the zenith: do not swing round
    return { key: 'sat:' + s.norad, name: s.name === 'ISS (ZARYA)' ? 'ISS' : s.name === 'CSS (TIANHE)' ? 'Tiangong' : s.name, alt: s.alt + (s.alt2 - s.alt) * k, az: (s.az + daz * k + 360) % 360, mag: s.mag, sunlit: s.sunlit,
      label: s.group === 'station' || s.mag < 1.5 }
  })
}

function SatSheet({ s, onClose }: { s: SatNow; onClose: () => void }) {
  const name = s.name === 'ISS (ZARYA)' ? 'International Space Station' : s.name === 'CSS (TIANHE)' ? 'Tiangong space station' : s.name
  const dir = ((s.az2 - s.az + 540) % 360) - 180
  return (
    <div className="sky-sheet" role="dialog" aria-label={name}>
      <div className="sky-sheet-head">
        <div className="grow"><div className="sky-sheet-title">{name}</div><div className="muted small">{s.group === 'station' ? 'Space station' : s.group === 'starlink' ? 'Starlink satellite' : 'Satellite'} · {s.km.toLocaleString()} km away</div></div>
        <button type="button" className="btn ghost icon-only" onClick={onClose} aria-label="Close"><Icon name="x" size={18} /></button>
      </div>
      <div className="sky-sheet-now"><span className="pill ok">{Math.round(s.alt)}° up</span><span>in the {direction(s.az)}, moving {dir >= 0 ? 'clockwise' : 'anticlockwise'} towards the {direction(s.az2 + (s.az2 - s.az) * 3)}</span></div>
      <p className="small">{s.sunlit ? `Lit by the Sun: magnitude ${s.mag}, ${s.mag < 0 ? 'brighter than almost every star' : s.mag < 2 ? 'easy to see' : 'faint'}. It crosses the sky in a few minutes without blinking (a blinking light is a plane).` : 'In the Earth\'s shadow now, so it cannot be seen.'}</p>
    </div>
  )
}

// ---- comets and asteroids ----

interface MinorObj { key: string; name: string; kind: string; mag: number; ra: number; dec: number; alt: number; az: number; con_name: string; dist_au: number; sun_au: number; best_at: string | null; dir: string; kit: string; words: string }

function MinorSheet({ m, tz, onClose, onPath }: { m: MinorObj; tz?: string; onClose: () => void; onPath: () => void }) {
  return (
    <div className="sky-sheet" role="dialog" aria-label={m.name}>
      <div className="sky-sheet-head">
        <div className="grow"><div className="sky-sheet-title">{m.name}</div><div className="muted small">{m.kind === 'comet' ? 'Comet' : 'Asteroid'} · in {m.con_name} · magnitude {m.mag}</div></div>
        <button type="button" className="btn ghost icon-only" onClick={onClose} aria-label="Close"><Icon name="x" size={18} /></button>
      </div>
      <p className="small">{m.words}</p>
      <p className="small muted">{m.dist_au} AU from the Earth, {m.sun_au} AU from the Sun.{m.kind === 'comet' ? ' Comets often turn out brighter or fainter than forecast.' : ''}{m.best_at ? ` Best at ${fmtAt(m.best_at, tz)}.` : ''}</p>
      <div className="row" style={{ gap: 6 }}><button type="button" className="btn sm" onClick={onPath}><Icon name="route" size={14} />Path tonight</button></div>
    </div>
  )
}
