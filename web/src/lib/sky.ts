// The star map: the catalogue (loaded once from the server and kept), the
// projection of the sky onto the screen and the drawing of it on a canvas.
// One renderer serves the draggable map (stereographic, like a planetarium
// dome) and the phone held up to the sky (rectilinear, like the camera).
import { api, V1 } from './api'
import {
  type BodyKey, BODIES, NAMES, type Mat3, type Observer, type Vec3, apply, at, horizonMatrix, mul, precession, radec, transpose, vec, enu, altAz,
} from './astro'

// ---- the catalogue ----

export type StarRow = [number, number, number, number, number, string, string, string] // ra dec mag bv hip name bayer con
export interface Con { id: string; name: string; gen: string; rank: number; label: [number, number]; lines: [number, number][][]; about: string }
export interface DSO { id: string; ngc: string; name: string; type: string; kind: string; mag: number | null; dim: string; ra: number; dec: number; con: string; about: string }
interface RawCatalog { stars: StarRow[]; constellations: Con[]; messier: DSO[]; highlights: DSO[]; milkyway: { id: string; rings: [number, number][][] }[]; bounds: { id: string; polys: [number, number][][] }[] }

export interface Catalog {
  stars: StarRow[]
  starVec: Float64Array // x y z per star (J2000)
  starColor: string[]
  cons: Con[]
  conById: Map<string, Con>
  conLines: { con: string; vecs: Vec3[] }[]
  conLabel: Map<string, Vec3>
  dsos: (DSO & { key: string; v: Vec3 })[]
  milky: { level: number; rings: Vec3[][] }[]
  bounds: { id: string; ring: [number, number][]; box: [number, number, number, number] }[]
  named: Map<number, number> // hip → index
}

let catalogP: Promise<Catalog> | null = null

export function loadCatalog(): Promise<Catalog> {
  if (!catalogP) {
    catalogP = api.get<RawCatalog>(`${V1}/catalog`).then(prepare)
    catalogP.catch(() => { catalogP = null })
  }
  return catalogP
}

function bvColor(bv: number) {
  if (bv < -0.2) return '#9db4ff'
  if (bv < 0) return '#b2c6ff'
  if (bv < 0.3) return '#d6e0ff'
  if (bv < 0.6) return '#fbf8ff'
  if (bv < 0.85) return '#fff1df'
  if (bv < 1.2) return '#ffd9a8'
  if (bv < 1.6) return '#ffbd7a'
  return '#ffa65c'
}

function unwrap(p: [number, number][]): { ring: [number, number][]; box: [number, number, number, number] } {
  const out: [number, number][] = []
  let prev = 0
  p.forEach(([ra, dec], i) => {
    if (i > 0) { while (ra - prev > 180) ra -= 360; while (ra - prev < -180) ra += 360 }
    out.push([ra, dec]); prev = ra
  })
  if (out.length > 2 && Math.abs(out[out.length - 1][0] - out[0][0]) > 180) {
    const pole = out[0][1] < 0 ? -90 : 90
    out.push([out[out.length - 1][0], pole], [out[0][0], pole])
  }
  const box: [number, number, number, number] = [Infinity, -Infinity, Infinity, -Infinity]
  for (const [ra, dec] of out) { box[0] = Math.min(box[0], ra); box[1] = Math.max(box[1], ra); box[2] = Math.min(box[2], dec); box[3] = Math.max(box[3], dec) }
  return { ring: out, box }
}

function prepare(raw: RawCatalog): Catalog {
  const n = raw.stars.length
  const starVec = new Float64Array(n * 3)
  const starColor: string[] = new Array(n)
  const named = new Map<number, number>()
  raw.stars.forEach((s, i) => {
    const v = vec(s[0], s[1])
    starVec[i * 3] = v[0]; starVec[i * 3 + 1] = v[1]; starVec[i * 3 + 2] = v[2]
    starColor[i] = bvColor(s[3])
    named.set(s[4], i)
  })
  const conById = new Map(raw.constellations.map(c => [c.id, c]))
  const conLines = raw.constellations.flatMap(c => c.lines.map(l => ({ con: c.id, vecs: l.map(([ra, dec]) => vec(ra, dec)) })))
  const conLabel = new Map(raw.constellations.map(c => [c.id, vec(c.label[0], c.label[1])]))
  const dsos = [...raw.messier, ...raw.highlights].map(d => ({ ...d, key: 'dso:' + d.id, v: vec(d.ra, d.dec) }))
  const milky = raw.milkyway.map((m, i) => ({ level: i, rings: m.rings.map(r => r.map(([ra, dec]) => vec(ra, dec))) }))
  const bounds = raw.bounds.flatMap(b => b.polys.map(p => ({ id: b.id.replace(/[12]$/, ''), ...unwrap(p) })))
  return { stars: raw.stars, starVec, starColor, cons: raw.constellations, conById, conLines, conLabel, dsos, milky, bounds, named }
}

/** The constellation a J2000 position lies in. */
export function conAt(cat: Catalog, ra: number, dec: number): string {
  for (const b of cat.bounds) {
    if (dec < b.box[2] || dec > b.box[3]) continue
    for (const r of [ra, ra - 360, ra + 360, ra - 720]) {
      if (r < b.box[0] || r > b.box[1]) continue
      let inside = false
      const ring = b.ring
      for (let i = 0, j = ring.length - 1; i < ring.length; j = i++) {
        const [xi, yi] = ring[i], [xj, yj] = ring[j]
        if ((yi > dec) !== (yj > dec) && r < (xj - xi) * (dec - yi) / (yj - yi) + xi) inside = !inside
      }
      if (inside) return b.id
    }
  }
  return ''
}

export function starLabel(cat: Catalog, s: StarRow) {
  if (s[5]) return s[5]
  const c = cat.conById.get(s[7])
  if (s[6] && c) return `${s[6]} ${c.gen}`
  return `HIP ${s[4]}`
}

// ---- the view ----

export interface Layers { lines: boolean; conNames: boolean; starNames: boolean; dso: boolean; milkyWay: boolean; grid: boolean; equator: boolean; ecliptic: boolean }

export interface View {
  t: number
  obs: Observer
  /** where the view looks (east-north-up): forward, up, right */
  f: Vec3; u: Vec3; r: Vec3
  fov: number // degrees across the screen's height (map) or long side (camera)
  proj: 'stereo' | 'gnomonic'
  fovLong?: boolean
  camera: boolean // a camera picture is behind: no sky fill, lighter ground
  layers: Layers
  magLimit: number
  selected?: string
}

/** The view frame for looking at an altitude and azimuth with no tilt. */
export function frameOf(alt: number, az: number): { f: Vec3; u: Vec3; r: Vec3 } {
  const f = enu(alt, az)
  const u = enu(alt + 90, az)
  return { f, u, r: cross(f, u) }
}

export const cross = (a: Vec3, b: Vec3): Vec3 => [a[1] * b[2] - a[2] * b[1], a[2] * b[0] - a[0] * b[2], a[0] * b[1] - a[1] * b[0]]
export const dot = (a: Vec3, b: Vec3) => a[0] * b[0] + a[1] * b[1] + a[2] * b[2]
export const norm = (a: Vec3): Vec3 => { const l = Math.hypot(a[0], a[1], a[2]) || 1; return [a[0] / l, a[1] / l, a[2] / l] }

/** Cuts a polygon (camera coordinates) to the part in front of the plane z = zmin (Sutherland–Hodgman). */
function clipZ(pts: Vec3[], zmin: number): Vec3[] {
  const out: Vec3[] = []
  for (let i = 0; i < pts.length; i++) {
    const a = pts[i], b = pts[(i + 1) % pts.length]
    const ina = a[2] >= zmin, inb = b[2] >= zmin
    if (ina) out.push(a)
    if (ina !== inb) {
      const t = (zmin - a[2]) / (b[2] - a[2])
      out.push([a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t, zmin])
    }
  }
  return out
}

export interface Hit { key: string; name: string; kind: 'body' | 'star' | 'dso' | 'constellation'; x: number; y: number; d: number }

export interface Painted {
  hits: { key: string; name: string; kind: Hit['kind']; x: number; y: number; r: number }[]
  /** screen → the sky (J2000 ra/dec, alt/az) */
  unproject: (x: number, y: number) => { ra: number; dec: number; alt: number; az: number } | null
  project: (enuV: Vec3) => [number, number, number] | null
  sunAlt: number
}

const PLANET_COLOR: Record<string, string> = { mercury: '#d2c2b0', venus: '#fff4d6', mars: '#ff8a5c', jupiter: '#f3dcb1', saturn: '#ecd795', uranus: '#b2ecf2', neptune: '#8fa8ff' }
const ECL = 23.4393

/** Draws the sky into a 2D context of css size w × h (already scaled for the pixel ratio). */
export function draw(ctx: CanvasRenderingContext2D, w: number, h: number, cat: Catalog | null, v: View): Painted {
  const P = precession(v.t)
  const H = horizonMatrix(v.obs, v.t)
  const C: Mat3 = [v.r, v.u, v.f]
  const CH = mul(C, H)          // equator of date → camera
  const CHP = mul(CH, P)        // J2000 → camera
  const stereo = v.proj === 'stereo'
  const span = v.fovLong ? Math.max(w, h) : h
  const half = (v.fov / 2) * Math.PI / 180
  const F = stereo ? (span / 2) / (2 * Math.tan(half / 2)) : (span / 2) / Math.tan(half)
  const cx = w / 2, cy = h / 2
  const minZ = stereo ? -0.6 : 0.02
  const out: Painted['hits'] = []
  // projection of camera coordinates; returns NaN when behind
  const px = (x: number, y: number, z: number): [number, number] | null => {
    if (z < minZ) return null
    if (stereo) { const k = 2 / (1 + z); return [cx + F * k * x, cy - F * k * y] }
    return [cx + F * x / z, cy - F * y / z]
  }
  const pxClamp = (x: number, y: number, z: number): [number, number] => {
    if (stereo) { const k = 2 / (1 + Math.max(z, -0.9)); return [cx + F * k * x, cy - F * k * y] }
    const zz = Math.max(z, 0.02)
    return [cx + F * x / zz, cy - F * y / zz]
  }
  const clipMin = stereo ? -0.85 : 0.03
  /** adds a polygon (camera coordinates) to the current path, cut to what is in front */
  const polyPath = (cam: Vec3[]) => {
    const c = cam.some(v => v[2] < clipMin) ? clipZ(cam, clipMin) : cam
    if (c.length < 3) return
    const p0 = pxClamp(c[0][0], c[0][1], c[0][2])
    ctx.moveTo(p0[0], p0[1])
    for (let k = 1; k < c.length; k++) { const p = pxClamp(c[k][0], c[k][1], c[k][2]); ctx.lineTo(p[0], p[1]) }
    ctx.closePath()
  }
  const onScreen = (p: [number, number] | null, m = 40) => !!p && p[0] > -m && p[0] < w + m && p[1] > -m && p[1] < h + m
  const camOf = (m: Mat3, vv: Vec3) => apply(m, vv)

  // the Sun's altitude sets the sky's light
  const sun = at('sun', v.obs, v.t)
  const moon = at('moon', v.obs, v.t)
  const s = sun.alt
  let limit = v.magLimit
  if (s > -18) limit = Math.min(limit, s > 0 ? -1.5 : s > -6 ? 1.2 + (-s / 6) * 1.3 : s > -12 ? 2.5 + ((-s - 6) / 6) * 1.6 : 4.1 + ((-s - 12) / 6) * 1.4)
  if (moon.alt > 0) limit -= moon.phase * 1.3
  limit = Math.min(limit, v.magLimit)

  // ---- background ----
  if (!v.camera) {
    const top = s > 0 ? '#3b78c9' : s > -6 ? '#243f74' : s > -12 ? '#111f40' : s > -18 ? '#0a1430' : '#050913'
    const low = s > 0 ? '#9cc3ef' : s > -6 ? '#5b5f86' : s > -12 ? '#22305a' : s > -18 ? '#111d3c' : '#0b1226'
    // gradient towards the horizon: find where the horizon is on screen
    const g = ctx.createLinearGradient(0, 0, 0, h)
    const hz = horizonY()
    g.addColorStop(0, top)
    g.addColorStop(Math.max(0.05, Math.min(0.98, hz)), low)
    g.addColorStop(1, low)
    ctx.fillStyle = g
    ctx.fillRect(0, 0, w, h)
  } else {
    ctx.clearRect(0, 0, w, h)
  }

  function horizonY() {
    // where the point straight ahead on the horizon lands, as a share of the height
    const fwd = norm([v.f[0], v.f[1], 0])
    const c = camOf(C, fwd)
    const p = px(c[0], c[1], c[2])
    return p ? p[1] / h : v.f[2] > 0 ? 1 : 0
  }

  const night = s < -10
  // ---- the Milky Way ----
  if (cat && v.layers.milkyWay && s < -9) {
    const a = (s < -16 ? 1 : (-s - 9) / 7) * (moon.alt > 0 ? 1 - moon.phase * 0.6 : 1) * (v.camera ? 0.55 : 1)
    for (const m of cat.milky) {
      ctx.fillStyle = `rgba(170, 190, 255, ${(0.035 + m.level * 0.012) * a})`
      ctx.beginPath()
      for (const ring of m.rings) polyPath(ring.map(vv => camOf(CHP, vv)))
      ctx.fill('evenodd')
    }
  }

  // ---- lines: grid, equator, ecliptic ----
  const line = (pts: Vec3[], m: Mat3, style: string, width = 1, dash: number[] = []) => {
    ctx.strokeStyle = style; ctx.lineWidth = width; ctx.setLineDash(dash)
    ctx.beginPath()
    let pen = false
    for (const vv of pts) {
      const c = camOf(m, vv)
      const p = px(c[0], c[1], c[2])
      if (!p || Math.abs(p[0] - cx) > w * 3 || Math.abs(p[1] - cy) > h * 3) { pen = false; continue }
      if (!pen) { ctx.moveTo(p[0], p[1]); pen = true } else ctx.lineTo(p[0], p[1])
    }
    ctx.stroke(); ctx.setLineDash([])
  }
  const I3: Mat3 = [[1, 0, 0], [0, 1, 0], [0, 0, 1]]
  if (v.layers.grid) {
    const gs = v.camera ? 'rgba(120, 200, 255, 0.35)' : 'rgba(110, 170, 255, 0.22)'
    for (let alt = 15; alt < 90; alt += 15) { const pts: Vec3[] = []; for (let az = 0; az <= 360; az += 2) pts.push(enu(alt, az)); line(pts, C, gs) }
    for (let az = 0; az < 360; az += 30) { const pts: Vec3[] = []; for (let alt = 0; alt <= 90; alt += 2) pts.push(enu(alt, az)); line(pts, C, gs) }
  }
  if (v.layers.equator) {
    const pts: Vec3[] = []; for (let ra = 0; ra <= 360; ra += 2) pts.push(vec(ra, 0)); line(pts, CH, 'rgba(120, 220, 200, 0.4)', 1, [4, 4])
    for (let ra = 0; ra < 360; ra += 30) { const m: Vec3[] = []; for (let dec = -88; dec <= 88; dec += 4) m.push(vec(ra, dec)); line(m, CH, 'rgba(120, 220, 200, 0.16)') }
  }
  if (v.layers.ecliptic) {
    const pts: Vec3[] = []
    for (let lon = 0; lon <= 360; lon += 2) {
      const x = Math.cos(lon * Math.PI / 180), y = Math.sin(lon * Math.PI / 180)
      pts.push([x, y * Math.cos(ECL * Math.PI / 180), y * Math.sin(ECL * Math.PI / 180)])
    }
    line(pts, CH, 'rgba(255, 196, 92, 0.42)', 1, [6, 5])
  }

  // ---- constellations ----
  if (cat && v.layers.lines) {
    ctx.strokeStyle = v.camera ? 'rgba(150, 200, 255, 0.55)' : night ? 'rgba(120, 160, 230, 0.42)' : 'rgba(170, 200, 255, 0.3)'
    ctx.lineWidth = v.camera ? 1.3 : 1
    ctx.beginPath()
    for (const l of cat.conLines) {
      let prev: [number, number] | null = null
      for (const vv of l.vecs) {
        const c = camOf(CHP, vv)
        const p = px(c[0], c[1], c[2])
        if (p && prev && Math.abs(p[0] - prev[0]) < w * 1.5 && Math.abs(p[1] - prev[1]) < h * 1.5) { ctx.moveTo(prev[0], prev[1]); ctx.lineTo(p[0], p[1]) }
        prev = p
      }
    }
    ctx.stroke()
  }

  // ---- stars ----
  const zoom = Math.max(0.85, Math.min(2.4, Math.sqrt(60 / v.fov)))
  const labels: { x: number; y: number; text: string; font: string; color: string; pri: number; dx?: number; dy?: number; align?: CanvasTextAlign }[] = []
  if (cat) {
    const m = CHP
    const sv = cat.starVec
    const lim = limit + Math.min(1.2, Math.log2(Math.max(1, 60 / v.fov)) * 0.9)
    for (let i = 0; i < cat.stars.length; i++) {
      const st = cat.stars[i]
      const mag = st[2]
      if (mag > lim) break // sorted by brightness
      const x = sv[i * 3], y = sv[i * 3 + 1], z = sv[i * 3 + 2]
      const X = m[0][0] * x + m[0][1] * y + m[0][2] * z
      const Y = m[1][0] * x + m[1][1] * y + m[1][2] * z
      const Z = m[2][0] * x + m[2][1] * y + m[2][2] * z
      if (Z < minZ) continue
      let sx: number, sy: number
      if (stereo) { const k = 2 / (1 + Z); sx = cx + F * k * X; sy = cy - F * k * Y } else { sx = cx + F * X / Z; sy = cy - F * Y / Z }
      if (sx < -8 || sx > w + 8 || sy < -8 || sy > h + 8) continue
      const r = Math.max(0.55, (0.5 + Math.max(0, lim - mag) * 0.5) * zoom * (v.camera ? 1.15 : 1))
      const fade = Math.min(1, (lim - mag) / 1.2 + 0.25)
      ctx.globalAlpha = fade
      ctx.fillStyle = cat.starColor[i]
      if (r < 1.2) ctx.fillRect(sx - r, sy - r, r * 2, r * 2)
      else {
        if (mag < 1.5 && night) { ctx.globalAlpha = 0.18 * fade; ctx.beginPath(); ctx.arc(sx, sy, r * 2.6, 0, Math.PI * 2); ctx.fill(); ctx.globalAlpha = fade }
        ctx.beginPath(); ctx.arc(sx, sy, r, 0, Math.PI * 2); ctx.fill()
      }
      if (st[5] || mag < 3.2) out.push({ key: 'star:' + st[4], name: starLabel(cat, st), kind: 'star', x: sx, y: sy, r: Math.max(r, 4) })
      if (v.layers.starNames && st[5] && (mag < 1.6 || (mag < 2.6 && v.fov < 80) || (mag < 3.6 && v.fov < 40))) {
        labels.push({ x: sx, y: sy, text: st[5], font: '11px system-ui, sans-serif', color: v.camera ? 'rgba(235,240,255,0.95)' : 'rgba(205,215,240,0.85)', pri: 10 - mag, dx: r + 4, dy: 3.5 })
      }
    }
    ctx.globalAlpha = 1
  }

  // ---- clusters, nebulae, galaxies ----
  if (cat && v.layers.dso) {
    const dsoLim = Math.min(limit + 2.5, v.fov < 40 ? 11 : v.fov < 80 ? 8.5 : 6.5)
    ctx.lineWidth = 1
    for (const d of cat.dsos) {
      if (d.mag == null || d.mag > dsoLim) continue
      const c = camOf(CHP, d.v)
      const p = px(c[0], c[1], c[2])
      if (!onScreen(p, 10)) continue
      const [x, y] = p!
      ctx.strokeStyle = v.camera ? 'rgba(255, 215, 150, 0.85)' : 'rgba(255, 205, 140, 0.7)'
      const r = 4.5 * Math.min(1.6, zoom)
      ctx.beginPath()
      if (/galaxy/i.test(d.kind)) ctx.ellipse(x, y, r * 1.35, r * 0.7, -0.5, 0, Math.PI * 2)
      else if (/nebula|remnant/i.test(d.kind)) ctx.rect(x - r, y - r, r * 2, r * 2)
      else if (/globular/i.test(d.kind)) { ctx.arc(x, y, r, 0, Math.PI * 2); ctx.moveTo(x - r, y); ctx.lineTo(x + r, y); ctx.moveTo(x, y - r); ctx.lineTo(x, y + r) }
      else { ctx.setLineDash([2, 2]); ctx.arc(x, y, r, 0, Math.PI * 2) }
      ctx.stroke(); ctx.setLineDash([])
      out.push({ key: d.key, name: d.id.startsWith('M') ? `${d.id}${d.name ? ' ' + d.name : ''}` : d.name, kind: 'dso', x, y, r: r + 2 })
      if (v.fov < 75 || (d.mag < 5 && v.fov < 110)) {
        labels.push({ x, y, text: d.id.startsWith('M') ? d.id : d.name, font: '10.5px system-ui, sans-serif', color: 'rgba(255, 210, 150, 0.85)', pri: 4 - d.mag / 3, dx: r + 3, dy: 3.5 })
      }
    }
  }

  // ---- the Sun, the Moon and the planets ----
  const bodies = BODIES.map(b => ({ key: b, s: b === 'sun' ? sun : b === 'moon' ? moon : at(b, v.obs, v.t) }))
  const sunCam = camOf(CH, vec(sun.ra, sun.dec))
  for (const { key, s: b } of bodies.slice().reverse()) {
    const c = camOf(CH, vec(b.ra, b.dec))
    const p = px(c[0], c[1], c[2])
    if (!onScreen(p, 60)) continue
    const [x, y] = p!
    const kpx = stereo ? F * 2 / (1 + c[2]) : F / Math.max(c[2], 0.02)
    const discR = (b.size / 3600 / 2) * Math.PI / 180 * kpx
    if (key === 'sun') {
      const r = Math.max(7, discR)
      const g = ctx.createRadialGradient(x, y, r * 0.6, x, y, r * 4)
      g.addColorStop(0, 'rgba(255, 236, 170, 0.55)'); g.addColorStop(1, 'rgba(255, 236, 170, 0)')
      ctx.fillStyle = g; ctx.beginPath(); ctx.arc(x, y, r * 4, 0, Math.PI * 2); ctx.fill()
      ctx.fillStyle = '#ffe9a6'; ctx.beginPath(); ctx.arc(x, y, r, 0, Math.PI * 2); ctx.fill()
      out.push({ key, name: 'Sun', kind: 'body', x, y, r: r + 6 })
      labels.push({ x, y, text: 'Sun', font: '600 12px system-ui, sans-serif', color: '#ffe9a6', pri: 100, dx: r + 6, dy: 4 })
    } else if (key === 'moon') {
      const r = Math.max(8, discR)
      // the lit side faces the Sun on the screen
      const sp = px(sunCam[0] * 0.02 + c[0] * 0.98, sunCam[1] * 0.02 + c[1] * 0.98, sunCam[2] * 0.02 + c[2] * 0.98) || pxClamp(sunCam[0], sunCam[1], sunCam[2])
      const ang = Math.atan2(sp[1] - y, sp[0] - x)
      drawMoon(ctx, x, y, r, b.phase, ang, v.camera)
      out.push({ key, name: 'Moon', kind: 'body', x, y, r: r + 6 })
      labels.push({ x, y, text: 'Moon', font: '600 12px system-ui, sans-serif', color: '#f3efe2', pri: 99, dx: r + 6, dy: 4 })
    } else {
      const r = Math.max(discR, Math.max(2.6, Math.min(6.5, 3.4 - b.mag * 0.7)) * Math.min(1.5, zoom))
      if (b.mag > limit + 3 && key !== 'uranus' && key !== 'neptune') continue
      ctx.fillStyle = PLANET_COLOR[key]
      ctx.globalAlpha = 0.25; ctx.beginPath(); ctx.arc(x, y, r * 2.2, 0, Math.PI * 2); ctx.fill(); ctx.globalAlpha = 1
      ctx.beginPath(); ctx.arc(x, y, r, 0, Math.PI * 2); ctx.fill()
      if (key === 'saturn') { ctx.strokeStyle = PLANET_COLOR.saturn; ctx.lineWidth = 1.2; ctx.beginPath(); ctx.ellipse(x, y, r * 2.1, r * 0.7, -0.35, 0, Math.PI * 2); ctx.stroke() }
      out.push({ key, name: NAMES[key as BodyKey], kind: 'body', x, y, r: r + 6 })
      labels.push({ x, y, text: NAMES[key as BodyKey], font: '600 12px system-ui, sans-serif', color: PLANET_COLOR[key], pri: 50 - b.mag, dx: r + 5, dy: 4 })
    }
  }

  // ---- constellation names ----
  if (cat && v.layers.conNames) {
    for (const c of cat.cons) {
      if (c.rank > (v.fov > 100 ? 1 : v.fov > 60 ? 2 : 3)) continue
      const lv = cat.conLabel.get(c.id)!
      const cc = camOf(CHP, lv)
      const p = px(cc[0], cc[1], cc[2])
      if (!onScreen(p, 0)) continue
      labels.push({ x: p![0], y: p![1], text: c.name.toUpperCase(), font: '500 10.5px system-ui, sans-serif', color: v.camera ? 'rgba(180, 215, 255, 0.9)' : 'rgba(140, 175, 235, 0.72)', pri: 20 - c.rank * 5, align: 'center' })
    }
  }

  // ---- the ground ----
  {
    const ground = v.camera ? 'rgba(8, 14, 24, 0.28)' : s > 0 ? 'rgba(46, 58, 48, 0.96)' : 'rgba(10, 16, 14, 0.94)'
    ctx.fillStyle = ground
    ctx.beginPath()
    for (let alt = 0; alt > -90; alt -= 10) {
      for (let az = 0; az < 360; az += 10) {
        const q = [enu(alt, az), enu(alt, az + 10), enu(alt - 10, az + 10), enu(alt - 10, az)].map(e => camOf(C, e))
        if (q.every(c => c[2] < clipMin)) continue
        polyPath(q)
      }
    }
    ctx.fill('nonzero')
    const hz: Vec3[] = []
    for (let az = 0; az <= 360; az += 1) hz.push(enu(0, az))
    line(hz, C, v.camera ? 'rgba(140, 230, 180, 0.8)' : 'rgba(120, 190, 150, 0.7)', 1.4)
    // compass points
    const names = ['N', 'NE', 'E', 'SE', 'S', 'SW', 'W', 'NW']
    names.forEach((nm, i) => {
      const c = camOf(C, enu(0, i * 45))
      const p = px(c[0], c[1], c[2])
      if (!onScreen(p, 0)) return
      labels.push({ x: p![0], y: p![1], text: nm, font: `${i % 2 ? 500 : 700} ${i % 2 ? 11 : 13}px system-ui, sans-serif`, color: nm === 'N' ? '#ff8f8f' : v.camera ? '#d9ffe8' : '#b9e3c9', pri: 200, align: 'center', dy: 16 })
    })
  }

  // ---- labels, brightest first, never on top of each other ----
  labels.sort((a, b) => b.pri - a.pri)
  const placed: [number, number, number, number][] = []
  ctx.textBaseline = 'alphabetic'
  for (const l of labels) {
    ctx.font = l.font
    const tw = ctx.measureText(l.text).width
    const align = l.align || 'left'
    let x = l.x + (l.dx || 0)
    if (align === 'center') x = l.x - tw / 2
    const y = l.y + (l.dy || 0)
    const box: [number, number, number, number] = [x - 2, y - 11, x + tw + 2, y + 3]
    if (box[2] < 0 || box[0] > w || box[3] < 0 || box[1] > h) continue
    if (placed.some(p => p[0] < box[2] && box[0] < p[2] && p[1] < box[3] && box[1] < p[3])) continue
    placed.push(box)
    if (v.camera) { ctx.fillStyle = 'rgba(0,0,0,0.55)'; ctx.fillText(l.text, x + 1, y + 1) }
    ctx.fillStyle = l.color
    ctx.fillText(l.text, x, y)
  }

  // ---- the selected object ----
  if (v.selected) {
    const hit = out.find(o => o.key === v.selected)
    let p: [number, number] | null = hit ? [hit.x, hit.y] : null
    if (!p && cat) {
      const d = cat.dsos.find(x => x.key === v.selected)
      const con = v.selected.startsWith('con:') ? cat.conLabel.get(v.selected.slice(4)) : undefined
      const sv = v.selected.startsWith('star:') ? cat.named.get(Number(v.selected.slice(5))) : undefined
      const vv = d?.v || con || (sv != null ? [cat.starVec[sv * 3], cat.starVec[sv * 3 + 1], cat.starVec[sv * 3 + 2]] as Vec3 : undefined)
      if (vv) { const c = camOf(CHP, vv); p = px(c[0], c[1], c[2]) }
    }
    if (p && onScreen(p, 0)) {
      ctx.strokeStyle = '#7fd1ff'; ctx.lineWidth = 1.6
      const r = Math.max(12, (hit?.r || 8) + 6)
      ctx.beginPath(); ctx.arc(p[0], p[1], r, 0, Math.PI * 2); ctx.stroke()
      for (const a of [0, 90, 180, 270]) {
        const rad = a * Math.PI / 180
        ctx.beginPath(); ctx.moveTo(p[0] + Math.cos(rad) * (r + 3), p[1] + Math.sin(rad) * (r + 3)); ctx.lineTo(p[0] + Math.cos(rad) * (r + 9), p[1] + Math.sin(rad) * (r + 9)); ctx.stroke()
      }
    }
  }

  const Ct = transpose(C), Ht = transpose(H), Pt = transpose(P)
  return {
    hits: out,
    sunAlt: s,
    project: (e: Vec3) => { const c = camOf(C, e); const p = px(c[0], c[1], c[2]); return p ? [p[0], p[1], c[2]] : null },
    unproject: (x: number, y: number) => {
      const X = (x - cx) / F, Y = -(y - cy) / F
      let cam: Vec3
      if (stereo) { const s2 = X * X + Y * Y; const z = (4 - s2) / (4 + s2); cam = [X * (1 + z) / 2, Y * (1 + z) / 2, z] } else cam = norm([X, Y, 1])
      const e = apply(Ct, cam)
      const [alt, az] = altAz(e)
      const [ra, dec] = radec(apply(Pt, apply(Ht, e)))
      return { ra, dec, alt, az }
    },
  }
}

function drawMoon(ctx: CanvasRenderingContext2D, x: number, y: number, r: number, k: number, ang: number, camera: boolean) {
  ctx.save()
  ctx.translate(x, y)
  ctx.rotate(ang)
  ctx.fillStyle = camera ? 'rgba(60, 62, 70, 0.75)' : 'rgba(70, 74, 86, 0.9)'
  ctx.beginPath(); ctx.arc(0, 0, r, 0, Math.PI * 2); ctx.fill()
  ctx.fillStyle = '#f4f1e4'
  ctx.beginPath()
  ctx.arc(0, 0, r, -Math.PI / 2, Math.PI / 2, false)
  const rx = Math.abs(r * (1 - 2 * k))
  ctx.ellipse(0, 0, rx, r, 0, Math.PI / 2, -Math.PI / 2, k < 0.5)
  ctx.fill()
  ctx.restore()
  const g = ctx.createRadialGradient(x, y, r, x, y, r * 2.4)
  g.addColorStop(0, `rgba(240, 236, 220, ${0.18 * k})`); g.addColorStop(1, 'rgba(240, 236, 220, 0)')
  ctx.fillStyle = g; ctx.beginPath(); ctx.arc(x, y, r * 2.4, 0, Math.PI * 2); ctx.fill()
}

/** The nearest thing drawn to a point on the screen. */
export function hitTest(p: Painted, x: number, y: number, cat: Catalog | null): Hit | null {
  let best: Hit | null = null
  for (const h of p.hits) {
    const d = Math.hypot(h.x - x, h.y - y)
    const reach = Math.max(18, h.r + 10)
    const score = d - (h.kind === 'body' ? 8 : 0)
    if (d <= reach && (!best || score < best.d)) best = { key: h.key, name: h.name, kind: h.kind, x: h.x, y: h.y, d: score }
  }
  if (best || !cat) return best
  const sky = p.unproject(x, y)
  if (!sky) return null
  const id = conAt(cat, sky.ra, sky.dec)
  const c = cat.conById.get(id)
  return c ? { key: 'con:' + c.id, name: c.name, kind: 'constellation', x, y, d: 0 } : null
}

/** The direction (J2000) of an object of the catalogue, or of a body now. */
export function directionOf(cat: Catalog | null, key: string, o: Observer, t: number): { alt: number; az: number } | null {
  if ((BODIES as string[]).includes(key)) { const s = at(key as BodyKey, o, t); return { alt: s.alt, az: s.az } }
  if (!cat) return null
  let v: Vec3 | undefined
  if (key.startsWith('dso:')) v = cat.dsos.find(d => d.key === key)?.v
  else if (key.startsWith('con:')) v = cat.conLabel.get(key.slice(4))
  else if (key.startsWith('star:')) { const i = cat.named.get(Number(key.slice(5))); if (i != null) v = [cat.starVec[i * 3], cat.starVec[i * 3 + 1], cat.starVec[i * 3 + 2]] }
  if (!v) return null
  const e = apply(horizonMatrix(o, t), apply(precession(t), v))
  const [alt, az] = altAz(e)
  return { alt, az }
}
