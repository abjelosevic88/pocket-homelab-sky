// Close-ups for the info card: Jupiter's four big moons and the Red Spot,
// Saturn's rings, and a map of the Moon's face with what stands out tonight.
import { useCallback, useEffect, useRef, useState } from 'react'
import { api, V1 } from '../lib/api'
import { fmtAt, fmtDayAt } from '../lib/format'
import { Modal, Spinner } from './ui'

interface JMoon { name: string; x: number; y: number; behind: boolean; in_shadow: boolean; transit: boolean }
interface Detail { jupiter: { moons: JMoon[]; red_spot_transits: string[] }; saturn: { ring_tilt: number; ring_pa: number; words: string } }

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

export function PlanetDetail({ k, t, tz }: { k: 'jupiter' | 'saturn'; t: number; tz?: string }) {
  const [d, setD] = useState<Detail | null>(null)
  const [ref, W] = useWidth(320)
  const tq = Math.floor(t / 60000)
  useEffect(() => { api.get<Detail>(`${V1}/planets/detail?t=${tq * 60000}`).then(setD).catch(() => {}) }, [tq])
  if (!d) return <div ref={ref} />
  if (k === 'saturn') {
    const H = 70, rx = Math.min(W / 2 - 20, 90), b = Math.abs(d.saturn.ring_tilt) * Math.PI / 180
    const ry = Math.max(1.5, rx * Math.sin(b))
    return (
      <div ref={ref} className="planet-detail">
        <svg width={W} height={H} viewBox={`0 0 ${W} ${H}`} role="img" aria-label={`Saturn's rings tilted ${d.saturn.ring_tilt}°`}>
          <g transform={`translate(${W / 2} ${H / 2}) rotate(${-d.saturn.ring_pa})`}>
            <ellipse rx={rx} ry={ry} fill="none" stroke="#e6d39a" strokeWidth={5} opacity={0.85} />
            <circle r={rx * 0.42} fill="#e8c77d" />
            <path d={`M${-rx} 0 A${rx} ${ry} 0 0 0 ${rx} 0`} fill="none" stroke="#f4e3ad" strokeWidth={5} opacity={d.saturn.ring_tilt >= 0 ? 0.95 : 0} />
          </g>
        </svg>
        <p className="small">The rings are {d.saturn.words} (tilted {Math.abs(d.saturn.ring_tilt)}°).</p>
      </div>
    )
  }
  const H = 64, far = Math.max(14, ...d.jupiter.moons.map(m => Math.abs(m.x) + 1)), sc = (W / 2 - 14) / far
  const next = d.jupiter.red_spot_transits.filter(s => new Date(s).getTime() > t - 20 * 60000).slice(0, 2)
  return (
    <div ref={ref} className="planet-detail">
      <svg width={W} height={H} viewBox={`0 0 ${W} ${H}`} role="img" aria-label="Jupiter's four big moons">
        <line x1={8} x2={W - 8} y1={H / 2 - 6} y2={H / 2 - 6} stroke="currentColor" strokeOpacity={0.12} />
        <circle cx={W / 2} cy={H / 2 - 6} r={Math.max(5, sc)} fill="#e7c9a0" />
        <rect x={W / 2 - Math.max(5, sc)} y={H / 2 - 7.5} width={2 * Math.max(5, sc)} height={2} fill="#b98a5e" opacity={0.7} />
        {d.jupiter.moons.map(m => {
          const x = W / 2 + m.x * sc, y = H / 2 - 6 - m.y * sc
          const hidden = m.behind || m.in_shadow
          return <g key={m.name} opacity={hidden ? 0.35 : 1}>
            <circle cx={x} cy={y} r={2.6} fill="#fff" stroke="#555" strokeWidth={0.5} />
            <text x={x} y={H - 6} textAnchor="middle" fontSize={11} fill="currentColor">{m.name}</text>
          </g>
        })}
        <text x={8} y={12} fontSize={10.5} fill="currentColor" opacity={0.6}>east</text>
        <text x={W - 8} y={12} fontSize={10.5} fill="currentColor" opacity={0.6} textAnchor="end">west</text>
      </svg>
      <p className="small">{d.jupiter.moons.filter(m => m.behind || m.in_shadow).map(m => `${m.name} is ${m.behind ? 'behind Jupiter' : "in Jupiter's shadow"}`).join('; ') || 'All four moons are on show; binoculars held steady show them as tiny stars in a line.'}
        {d.jupiter.moons.some(m => m.transit) ? ` ${d.jupiter.moons.filter(m => m.transit).map(m => m.name).join(' and ')} crossing in front of the planet (a telescope shows its shadow).` : ''}</p>
      {next.length > 0 && <p className="small muted">The Great Red Spot faces us at {next.map(s => `${fmtDayAt(s, tz)} ${fmtAt(s, tz)}`).join(' and ')} (best an hour either side; a telescope of 10 cm or more).</p>}
    </div>
  )
}

// ---- the Moon's face ----

interface MoonFeature { name: string; kind: string; lat: number; lon: number; size_km: number; about: string }
interface MoonMapData { phase: number; age: number; colongitude: number; terminator: number; morning: boolean; libration: { lat: number; lon: number }; best: MoonFeature[]; features: MoonFeature[] }

export function MoonMapButton({ t }: { t: number }) {
  const [open, setOpen] = useState(false)
  return <>
    <button type="button" className="btn sm" onClick={() => setOpen(true)}>Moon map</button>
    {open && <MoonMap t={t} onClose={() => setOpen(false)} />}
  </>
}

const RAD = Math.PI / 180

function MoonMap({ t, onClose }: { t: number; onClose: () => void }) {
  const [d, setD] = useState<MoonMapData | null>(null)
  const [pick, setPick] = useState<MoonFeature | null>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const [ref, W] = useWidth(360)
  useEffect(() => { api.get<MoonMapData>(`${V1}/moon?t=${Math.floor(t / 60000) * 60000}`).then(setD).catch(() => {}) }, [t])
  const S = Math.min(W, 420)
  // orthographic: selenographic lat/lon → screen, seen from the Earth (with libration)
  const proj = useCallback((lat: number, lon: number, data: MoonMapData) => {
    const l0 = data.libration.lon * RAD, b0 = data.libration.lat * RAD, R = S / 2 - 6
    const la = lat * RAD, lo = lon * RAD
    const cosc = Math.sin(b0) * Math.sin(la) + Math.cos(b0) * Math.cos(la) * Math.cos(lo - l0)
    const x = R * Math.cos(la) * Math.sin(lo - l0)
    const y = R * (Math.cos(b0) * Math.sin(la) - Math.sin(b0) * Math.cos(la) * Math.cos(lo - l0))
    return { x: S / 2 + x, y: S / 2 - y, front: cosc > 0 }
  }, [S])
  useEffect(() => {
    const c = canvas.current
    if (!c || !d) return
    const dpr = Math.min(2, window.devicePixelRatio || 1)
    c.width = S * dpr; c.height = S * dpr; c.style.width = S + 'px'; c.style.height = S + 'px'
    const ctx = c.getContext('2d')!
    const img = ctx.createImageData(S * dpr, S * dpr)
    const R = (S / 2 - 6) * dpr, cx = S * dpr / 2
    const l0 = d.libration.lon * RAD, b0 = d.libration.lat * RAD
    const sunLon = (90 - d.colongitude) * RAD
    const seas = d.features.filter(f => f.kind === 'sea')
    for (let py = 0; py < S * dpr; py++) {
      for (let px = 0; px < S * dpr; px++) {
        const x = (px - cx) / R, y = -(py - cx) / R
        const r2 = x * x + y * y
        if (r2 > 1) continue
        const z = Math.sqrt(1 - r2)
        // back to selenographic coordinates (inverse orthographic)
        const lat = Math.asin(z * Math.sin(b0) + y * Math.cos(b0))
        const lon = l0 + Math.atan2(x, z * Math.cos(b0) - y * Math.sin(b0))
        const lit = Math.cos(lat) * Math.cos(lon - sunLon)
        let base = 205
        for (const s of seas) {
          const dd = Math.acos(Math.min(1, Math.sin(lat) * Math.sin(s.lat * RAD) + Math.cos(lat) * Math.cos(s.lat * RAD) * Math.cos(lon - s.lon * RAD))) * 1737
          if (dd < s.size_km / 2) { base = 150; break }
        }
        const shade = lit > 0 ? base * (0.55 + 0.45 * Math.min(1, lit * 3)) : 34
        const i = (py * S * dpr + px) * 4
        img.data[i] = shade; img.data[i + 1] = shade * 0.98; img.data[i + 2] = shade * 0.92; img.data[i + 3] = 255
      }
    }
    ctx.putImageData(img, 0, 0)
  }, [d, S])
  const best = new Set(d?.best.map(b => b.name) || [])
  return (
    <Modal title="The Moon tonight" onClose={onClose} size="lg">
      <div ref={ref} className="moon-map">
        {!d && <Spinner />}
        {d && <>
          <div className="moon-canvas" style={{ width: S, height: S }}>
            <canvas ref={canvas} />
            {d.features.map(f => {
              const p = proj(f.lat, f.lon, d)
              if (!p.front || (f.kind !== 'sea' && !best.has(f.name))) return null
              return <button key={f.name} type="button" className={`moon-label ${f.kind} ${best.has(f.name) ? 'best' : ''}`} style={{ left: p.x, top: p.y }} onClick={() => setPick(f)}>{f.name}</button>
            })}
          </div>
          <div className="moon-side">
            <p className="small">{Math.round(d.phase * 100)}% lit. The shadow line shows craters and mountains in sharp relief; these stand out tonight:</p>
            <div className="list bordered">
              {d.best.map(f => <button key={f.name} type="button" className="list-row" onClick={() => setPick(f)}><span className="row-main grow"><span className="row-title">{f.name}</span><span className="row-sub small muted">{f.kind}{f.size_km ? ` · ${Math.round(f.size_km)} km` : ''}</span></span></button>)}
            </div>
            {pick && <p className="small mt"><b>{pick.name}.</b> {pick.about}</p>}
            <p className="muted small mt">North is up, as you see it with your eyes or binoculars (a telescope may turn it round).</p>
          </div>
        </>}
      </div>
    </Modal>
  )
}
