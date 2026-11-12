// Equipment and what it shows: magnification, how much sky fits in the view,
// how faint it reaches. Setups are every useful combination of your kit.
export interface Kit {
  id: number; kind: 'binoculars' | 'telescope' | 'eyepiece' | 'camera'; name: string
  aperture: number; focal: number; afov: number; tfov: number; power: number; sensor_w: number; sensor_h: number; notes: string; created_at: string
}

export interface Setup {
  id: string
  label: string
  /** field of view in degrees: a circle (w only) or a camera's rectangle */
  w: number
  h?: number
  power?: number
  reach?: number // faintest magnitude, about
}

const DEG = 180 / Math.PI
const round = (v: number, d = 1) => Math.round(v * 10 ** d) / 10 ** d

/** About the faintest star a lens of this many mm shows (dark sky). */
export const reachOf = (aperture: number) => round(7.7 + 5 * Math.log10(aperture / 10), 1)

export function setups(kit: Kit[]): Setup[] {
  const out: Setup[] = []
  const scopes = kit.filter(k => k.kind === 'telescope')
  const eyes = kit.filter(k => k.kind === 'eyepiece')
  const cams = kit.filter(k => k.kind === 'camera')
  for (const b of kit.filter(k => k.kind === 'binoculars')) {
    out.push({ id: `b${b.id}`, label: `${b.name} (${b.power}×, ${round(b.tfov)}°)`, w: b.tfov, power: b.power, reach: reachOf(b.aperture) })
  }
  for (const t of scopes) {
    for (const e of eyes) {
      const p = t.focal / e.focal
      out.push({ id: `t${t.id}e${e.id}`, label: `${t.name} + ${e.name} (${Math.round(p)}×, ${round(e.afov / p, 2)}°)`, w: e.afov / p, power: p, reach: reachOf(t.aperture) })
    }
    for (const c of cams) {
      const w = 2 * Math.atan(c.sensor_w / (2 * t.focal)) * DEG, h = 2 * Math.atan(c.sensor_h / (2 * t.focal)) * DEG
      out.push({ id: `t${t.id}c${c.id}`, label: `${t.name} + ${c.name} (${round(w, 2)}° × ${round(h, 2)}°)`, w, h, reach: reachOf(t.aperture) })
    }
  }
  for (const c of cams) {
    if (c.focal > 0) {
      const w = 2 * Math.atan(c.sensor_w / (2 * c.focal)) * DEG, h = 2 * Math.atan(c.sensor_h / (2 * c.focal)) * DEG
      out.push({ id: `c${c.id}`, label: `${c.name}, ${c.focal} mm lens (${round(w)}° × ${round(h)}°)`, w, h })
    }
  }
  return out
}

const FOV_KEY = 'psky-fov'
export function savedFov(): Setup | null { try { return JSON.parse(localStorage.getItem(FOV_KEY) || 'null') } catch { return null } }
export function saveFov(s: Setup | null) { if (s) localStorage.setItem(FOV_KEY, JSON.stringify(s)); else localStorage.removeItem(FOV_KEY) }

/** Plain words about one piece of kit. */
export function kitWords(k: Kit): string {
  switch (k.kind) {
    case 'binoculars': return `${k.power}× with ${k.aperture} mm lenses: ${round(k.tfov)}° of sky, stars to about magnitude ${reachOf(k.aperture)}.`
    case 'telescope': return `${k.aperture} mm aperture, ${k.focal} mm focal length (f/${round(k.focal / k.aperture)}): stars to about magnitude ${reachOf(k.aperture)}, up to about ${Math.round(k.aperture * 2)}× on a steady night.`
    case 'eyepiece': return `${k.focal} mm, ${k.afov}° apparent field.`
    case 'camera': return `Sensor ${k.sensor_w} × ${k.sensor_h} mm${k.focal ? `, ${k.focal} mm lens` : ''}.`
  }
}
