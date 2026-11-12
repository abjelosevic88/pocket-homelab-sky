// Where the Sun, the Moon, the planets and the stars are: the same engine as
// the server (internal/astro, Paul Schlyter's method with the main
// perturbations), so the map, Tonight and the server agree. Both answer the
// vectors in internal/astro/testdata/vectors.json (scripts/astro-vectors.mjs).
// Angles are degrees, right ascension is degrees of the equator of date.

const RAD = Math.PI / 180
const DEG = 180 / Math.PI
const AU_KM = 149597870.7
const EARTH_R_KM = 6378.14

export type BodyKey = 'sun' | 'moon' | 'mercury' | 'venus' | 'mars' | 'jupiter' | 'saturn' | 'uranus' | 'neptune'
export const BODIES: BodyKey[] = ['sun', 'moon', 'mercury', 'venus', 'mars', 'jupiter', 'saturn', 'uranus', 'neptune']
export const PLANETS: BodyKey[] = ['mercury', 'venus', 'mars', 'jupiter', 'saturn', 'uranus', 'neptune']
export const NAMES: Record<BodyKey, string> = { sun: 'Sun', moon: 'Moon', mercury: 'Mercury', venus: 'Venus', mars: 'Mars', jupiter: 'Jupiter', saturn: 'Saturn', uranus: 'Uranus', neptune: 'Neptune' }
const RADIUS_KM: Record<BodyKey, number> = { sun: 696000, moon: 1737.4, mercury: 2439.7, venus: 6051.8, mars: 3396.2, jupiter: 71492, saturn: 60268, uranus: 25559, neptune: 24764 }

export const jd = (ms: number) => ms / 86400000 + 2440587.5
const dayNumber = (ms: number) => jd(ms) - 2451543.5

export function rev(x: number) { x %= 360; return x < 0 ? x + 360 : x }
export function wrap180(x: number) { x = rev(x); return x > 180 ? x - 360 : x }
const sind = (x: number) => Math.sin(x * RAD)
const cosd = (x: number) => Math.cos(x * RAD)
const atan2d = (y: number, x: number) => Math.atan2(y, x) * DEG
const clamp = (x: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, x))
const asind = (x: number) => Math.asin(clamp(x, -1, 1)) * DEG
const acosd = (x: number) => Math.acos(clamp(x, -1, 1)) * DEG

const obliquity = (d: number) => 23.4393 - 3.563e-7 * d

interface El { N: number; i: number; w: number; a: number; e: number; M: number }

function orbit(body: BodyKey, d: number): El {
  switch (body) {
    case 'sun': return { N: 0, i: 0, w: 282.9404 + 4.70935e-5 * d, a: 1, e: 0.016709 - 1.151e-9 * d, M: 356.0470 + 0.9856002585 * d }
    case 'moon': return { N: 125.1228 - 0.0529538083 * d, i: 5.1454, w: 318.0634 + 0.1643573223 * d, a: 60.2666, e: 0.054900, M: 115.3654 + 13.0649929509 * d }
    case 'mercury': return { N: 48.3313 + 3.24587e-5 * d, i: 7.0047 + 5.00e-8 * d, w: 29.1241 + 1.01444e-5 * d, a: 0.387098, e: 0.205635 + 5.59e-10 * d, M: 168.6562 + 4.0923344368 * d }
    case 'venus': return { N: 76.6799 + 2.46590e-5 * d, i: 3.3946 + 2.75e-8 * d, w: 54.8910 + 1.38374e-5 * d, a: 0.723330, e: 0.006773 - 1.302e-9 * d, M: 48.0052 + 1.6021302244 * d }
    case 'mars': return { N: 49.5574 + 2.11081e-5 * d, i: 1.8497 - 1.78e-8 * d, w: 286.5016 + 2.92961e-5 * d, a: 1.523688, e: 0.093405 + 2.516e-9 * d, M: 18.6021 + 0.5240207766 * d }
    case 'jupiter': return { N: 100.4542 + 2.76854e-5 * d, i: 1.3030 - 1.557e-7 * d, w: 273.8777 + 1.64505e-5 * d, a: 5.20256, e: 0.048498 + 4.469e-9 * d, M: 19.8950 + 0.0830853001 * d }
    case 'saturn': return { N: 113.6634 + 2.38980e-5 * d, i: 2.4886 - 1.081e-7 * d, w: 339.3939 + 2.97661e-5 * d, a: 9.55475, e: 0.055546 - 9.499e-9 * d, M: 316.9670 + 0.0334442282 * d }
    case 'uranus': return { N: 74.0005 + 1.3978e-5 * d, i: 0.7733 + 1.9e-8 * d, w: 96.6612 + 3.0565e-5 * d, a: 19.18171 - 1.55e-8 * d, e: 0.047318 + 7.45e-9 * d, M: 142.5905 + 0.011725806 * d }
    case 'neptune': return { N: 131.7806 + 3.0173e-5 * d, i: 1.7700 - 2.55e-7 * d, w: 272.8461 - 6.027e-6 * d, a: 30.05826 + 3.313e-8 * d, e: 0.008606 + 2.15e-9 * d, M: 260.2471 + 0.005995147 * d }
  }
}

function kepler(M: number, e: number) {
  M = rev(M)
  let E = M + e * DEG * sind(M) * (1 + e * cosd(M))
  for (let k = 0; k < 30; k++) {
    const dE = (E - e * DEG * sind(E) - M) / (1 - e * cosd(E))
    E -= dE
    if (Math.abs(dE) < 1e-9) break
  }
  return E
}

function inOrbit(el: El): [number, number] {
  const E = kepler(el.M, el.e)
  const xv = el.a * (cosd(E) - el.e)
  const yv = el.a * (Math.sqrt(1 - el.e * el.e) * sind(E))
  return [atan2d(yv, xv), Math.hypot(xv, yv)]
}

function eclipticOf(el: El): [number, number, number] {
  const [v, r] = inOrbit(el)
  const vw = v + el.w
  const x = r * (cosd(el.N) * cosd(vw) - sind(el.N) * sind(vw) * cosd(el.i))
  const y = r * (sind(el.N) * cosd(vw) + cosd(el.N) * sind(vw) * cosd(el.i))
  const z = r * (sind(vw) * sind(el.i))
  return [rev(atan2d(y, x)), atan2d(z, Math.hypot(x, y)), r]
}

function sunEcliptic(d: number): [number, number] {
  const el = orbit('sun', d)
  const [v, r] = inOrbit(el)
  return [rev(v + el.w), r]
}

function moonEcliptic(d: number): [number, number, number] {
  const m = orbit('moon', d), s = orbit('sun', d)
  let [lon, lat, r] = eclipticOf(m)
  const Ms = rev(s.M), Mm = rev(m.M)
  const Ls = rev(s.M + s.w), Lm = rev(m.M + m.w + m.N)
  const D = Lm - Ls, F = Lm - m.N
  lon += -1.274 * sind(Mm - 2 * D) + 0.658 * sind(2 * D) - 0.186 * sind(Ms) - 0.059 * sind(2 * Mm - 2 * D) - 0.057 * sind(Mm - 2 * D + Ms) +
    0.053 * sind(Mm + 2 * D) + 0.046 * sind(2 * D - Ms) + 0.041 * sind(Mm - Ms) - 0.035 * sind(D) - 0.031 * sind(Mm + Ms) -
    0.015 * sind(2 * F - 2 * D) + 0.011 * sind(Mm - 4 * D)
  lat += -0.173 * sind(F - 2 * D) - 0.055 * sind(Mm - F - 2 * D) - 0.046 * sind(Mm + F - 2 * D) + 0.033 * sind(F + 2 * D) + 0.017 * sind(2 * Mm + F)
  r += -0.58 * cosd(Mm - 2 * D) - 0.46 * cosd(2 * D)
  return [rev(lon), lat, r]
}

function helioPlanet(body: BodyKey, d: number): [number, number, number] {
  let [lon, lat, r] = eclipticOf(orbit(body, d))
  const Mj = rev(orbit('jupiter', d).M), Ms = rev(orbit('saturn', d).M), Mu = rev(orbit('uranus', d).M)
  if (body === 'jupiter') {
    lon += -0.332 * sind(2 * Mj - 5 * Ms - 67.6) - 0.056 * sind(2 * Mj - 2 * Ms + 21) + 0.042 * sind(3 * Mj - 5 * Ms + 21) -
      0.036 * sind(Mj - 2 * Ms) + 0.022 * cosd(Mj - Ms) + 0.023 * sind(2 * Mj - 3 * Ms + 52) - 0.016 * sind(Mj - 5 * Ms - 69)
  } else if (body === 'saturn') {
    lon += 0.812 * sind(2 * Mj - 5 * Ms - 67.6) - 0.229 * cosd(2 * Mj - 4 * Ms - 2) + 0.119 * sind(Mj - 2 * Ms - 3) +
      0.046 * sind(2 * Mj - 6 * Ms - 69) + 0.014 * sind(Mj - 3 * Ms + 32)
    lat += -0.020 * cosd(2 * Mj - 4 * Ms - 2) + 0.018 * sind(2 * Mj - 6 * Ms - 49)
  } else if (body === 'uranus') {
    lon += 0.040 * sind(Ms - 2 * Mu + 6) + 0.035 * sind(Ms - 3 * Mu + 33) - 0.015 * sind(Mj - Mu + 20)
  }
  return [rev(lon), lat, r]
}

export interface Position {
  body: BodyKey
  ra: number; dec: number
  lon: number; lat: number
  dist: number   // AU from the Earth
  helio: number
  mag: number
  phase: number  // lit fraction
  elong: number
  size: number   // arcseconds
}

function eclToEqu(lon: number, lat: number, ecl: number): [number, number] {
  const x = cosd(lon) * cosd(lat), y = sind(lon) * cosd(lat), z = sind(lat)
  const ye = y * cosd(ecl) - z * sind(ecl)
  const ze = y * sind(ecl) + z * cosd(ecl)
  return [rev(atan2d(ye, x)), atan2d(ze, Math.hypot(x, ye))]
}

function planetMag(name: BodyKey, r: number, R: number, fv: number, glon: number, glat: number, d: number) {
  const l = 5 * Math.log10(r * R)
  switch (name) {
    case 'mercury': return -0.36 + l + 0.027 * fv + 2.2e-13 * Math.pow(fv, 6)
    case 'venus': return -4.34 + l + 0.013 * fv + 4.2e-7 * Math.pow(fv, 3)
    case 'mars': return -1.51 + l + 0.016 * fv
    case 'jupiter': return -9.25 + l + 0.014 * fv
    case 'saturn': {
      const ir = 28.06, Nr = 169.51 + 3.82e-5 * d
      const B = asind(sind(glat) * cosd(ir) - cosd(glat) * sind(ir) * sind(glon - Nr))
      return -9.0 + l + 0.044 * fv - 2.6 * sind(Math.abs(B)) + 1.2 * sind(B) * sind(B)
    }
    case 'uranus': return -7.15 + l + 0.001 * fv
    case 'neptune': return -6.90 + l + 0.001 * fv
  }
  return 0
}

/** A body at a moment, from the centre of the Earth (t in ms). */
export function body(name: BodyKey, t: number): Position {
  const d = dayNumber(t)
  const ecl = obliquity(d)
  const [slon, sr] = sunEcliptic(d)
  const p: Position = { body: name, ra: 0, dec: 0, lon: 0, lat: 0, dist: 0, helio: 0, mag: 0, phase: 0, elong: 0, size: 0 }
  if (name === 'sun') {
    p.lon = slon; p.dist = sr; p.mag = -26.74; p.phase = 1
  } else if (name === 'moon') {
    const [lon, lat, rer] = moonEcliptic(d)
    p.lon = lon; p.lat = lat; p.dist = rer * EARTH_R_KM / AU_KM
    p.elong = acosd(cosd(lon - slon) * cosd(lat))
    const fv = Math.abs(atan2d(sr * sind(p.elong), p.dist - sr * cosd(p.elong)))
    p.phase = (1 + cosd(fv)) / 2
    p.helio = sr
    p.mag = -12.73 + 0.026 * fv + 4e-9 * Math.pow(fv, 4)
  } else {
    const [lon, lat, r] = helioPlanet(name, d)
    const xh = r * cosd(lon) * cosd(lat), yh = r * sind(lon) * cosd(lat), zh = r * sind(lat)
    const xg = xh + sr * cosd(slon), yg = yh + sr * sind(slon)
    const R = Math.sqrt(xg * xg + yg * yg + zh * zh)
    p.lon = rev(atan2d(yg, xg)); p.lat = atan2d(zh, Math.hypot(xg, yg)); p.dist = R; p.helio = r
    p.elong = acosd((sr * sr + R * R - r * r) / (2 * sr * R))
    const fv = acosd((r * r + R * R - sr * sr) / (2 * r * R))
    p.phase = (1 + cosd(fv)) / 2
    p.mag = planetMag(name, r, R, fv, p.lon, p.lat, d)
  }
  ;[p.ra, p.dec] = eclToEqu(p.lon, p.lat, ecl)
  if (p.dist > 0) p.size = 2 * Math.atan(RADIUS_KM[name] / (p.dist * AU_KM)) * DEG * 3600
  return p
}

export interface Observer { lat: number; lon: number }

/** Local mean sidereal time, degrees. */
export function siderealTime(t: number, lon: number) {
  const j = jd(t)
  const T = (j - 2451545.0) / 36525
  return rev(280.46061837 + 360.98564736629 * (j - 2451545.0) + 0.000387933 * T * T - T * T * T / 38710000 + lon)
}

/** The position seen from a place (the Moon moves up to a degree). */
export function topocentric(p: Position, o: Observer, t: number): Position {
  if (p.dist <= 0) return p
  const par = asind(EARTH_R_KM / (p.dist * AU_KM))
  if (par < 1e-6) return p
  const gclat = o.lat - 0.1924 * sind(2 * o.lat)
  const rho = 0.99833 + 0.00167 * cosd(2 * o.lat)
  const ha = siderealTime(t, o.lon) - p.ra
  const rcos = rho * cosd(gclat), rsin = rho * sind(gclat), sp = sind(par)
  const dA = atan2d(-rcos * sp * sind(ha), cosd(p.dec) - rcos * sp * cosd(ha))
  const q = { ...p }
  q.ra = rev(p.ra + dA)
  q.dec = atan2d((sind(p.dec) - rsin * sp) * cosd(dA), cosd(p.dec) - rcos * sp * cosd(ha))
  const x = p.dist * cosd(p.dec) * cosd(ha) - rcos * EARTH_R_KM / AU_KM
  const y = p.dist * cosd(p.dec) * sind(ha)
  const z = p.dist * sind(p.dec) - rsin * EARTH_R_KM / AU_KM
  q.dist = Math.sqrt(x * x + y * y + z * z)
  q.size = 2 * Math.atan(RADIUS_KM[p.body] / (q.dist * AU_KM)) * DEG * 3600
  return q
}

/** Altitude and azimuth (from north through east) of a point of the sky. */
export function horizontal(ra: number, dec: number, o: Observer, t: number): [number, number] {
  const ha = siderealTime(t, o.lon) - ra
  const alt = asind(sind(o.lat) * sind(dec) + cosd(o.lat) * cosd(dec) * cosd(ha))
  const az = rev(atan2d(-cosd(dec) * sind(ha), sind(dec) * cosd(o.lat) - cosd(dec) * sind(o.lat) * cosd(ha)))
  return [alt, az]
}

export interface Seen extends Position { alt: number; az: number }

/** A body seen from a place at a moment. */
export function at(name: BodyKey, o: Observer, t: number): Seen {
  const p = topocentric(body(name, t), o, t)
  const [alt, az] = horizontal(p.ra, p.dec, o, t)
  return { ...p, alt, az }
}

/** Angle between two points of the sky (degrees). */
export function separation(ra1: number, dec1: number, ra2: number, dec2: number) {
  const dra = (ra2 - ra1) * RAD, p1 = dec1 * RAD, p2 = dec2 * RAD
  const a = Math.cos(p2) * Math.sin(dra)
  const b = Math.cos(p1) * Math.sin(p2) - Math.sin(p1) * Math.cos(p2) * Math.cos(dra)
  const c = Math.sin(p1) * Math.sin(p2) + Math.cos(p1) * Math.cos(p2) * Math.cos(dra)
  return Math.atan2(Math.hypot(a, b), c) * DEG
}

// ---- vectors and rotations ----

export type Vec3 = [number, number, number]
export type Mat3 = [Vec3, Vec3, Vec3]

export const vec = (ra: number, dec: number): Vec3 => [cosd(dec) * cosd(ra), cosd(dec) * sind(ra), sind(dec)]
export const radec = (v: Vec3): [number, number] => [rev(atan2d(v[1], v[0])), atan2d(v[2], Math.hypot(v[0], v[1]))]
export const apply = (m: Mat3, v: Vec3): Vec3 => [
  m[0][0] * v[0] + m[0][1] * v[1] + m[0][2] * v[2],
  m[1][0] * v[0] + m[1][1] * v[1] + m[1][2] * v[2],
  m[2][0] * v[0] + m[2][1] * v[1] + m[2][2] * v[2],
]
export const transpose = (m: Mat3): Mat3 => [[m[0][0], m[1][0], m[2][0]], [m[0][1], m[1][1], m[2][1]], [m[0][2], m[1][2], m[2][2]]]
export function mul(a: Mat3, b: Mat3): Mat3 {
  const o: Mat3 = [[0, 0, 0], [0, 0, 0], [0, 0, 0]]
  for (let i = 0; i < 3; i++) for (let j = 0; j < 3; j++) o[i][j] = a[i][0] * b[0][j] + a[i][1] * b[1][j] + a[i][2] * b[2][j]
  return o
}

/** J2000 → equator and equinox of date (Meeus 21). */
export function precession(t: number): Mat3 {
  const T = (jd(t) - 2451545.0) / 36525
  const zeta = (2306.2181 * T + 0.30188 * T * T + 0.017998 * T * T * T) / 3600
  const z = (2306.2181 * T + 1.09468 * T * T + 0.018203 * T * T * T) / 3600
  const th = (2004.3109 * T - 0.42665 * T * T - 0.041833 * T * T * T) / 3600
  const cz = cosd(zeta), sz = sind(zeta), cZ = cosd(z), sZ = sind(z), ct = cosd(th), st = sind(th)
  return [
    [cZ * ct * cz - sZ * sz, -cZ * ct * sz - sZ * cz, -cZ * st],
    [sZ * ct * cz + cZ * sz, -sZ * ct * sz + cZ * cz, -sZ * st],
    [st * cz, -st * sz, ct],
  ]
}

/** Equator of date → the horizon at a place (east, north, up). */
export function horizonMatrix(o: Observer, t: number): Mat3 {
  const th = siderealTime(t, o.lon)
  const ct = cosd(th), st = sind(th), cp = cosd(o.lat), sp = sind(o.lat)
  return [
    [-st, ct, 0],
    [-sp * ct, -sp * st, cp],
    [cp * ct, cp * st, sp],
  ]
}

/** East-north-up vector of an altitude and azimuth. */
export const enu = (alt: number, az: number): Vec3 => [cosd(alt) * sind(az), cosd(alt) * cosd(az), sind(alt)]
/** Altitude and azimuth of an east-north-up vector. */
export const altAz = (v: Vec3): [number, number] => [atan2d(v[2], Math.hypot(v[0], v[1])), rev(atan2d(v[0], v[1]))]

// ---- rising and setting ----

export const HORIZON_SUN = -0.833
export const HORIZON_POINT = -0.5667
export const horizonOf = (b: BodyKey) => (b === 'sun' || b === 'moon' ? HORIZON_SUN : HORIZON_POINT)

export interface Crossing { t: number; rising: boolean }

/** Every time f passes level between from and to (ms), to the second. */
export function crossings(f: (t: number) => number, level: number, from: number, to: number, step = 600000): Crossing[] {
  const out: Crossing[] = []
  let t0 = from, a0 = f(t0) - level
  while (t0 < to) {
    const t1 = Math.min(to, t0 + step)
    const a1 = f(t1) - level
    if ((a0 < 0) !== (a1 < 0)) {
      let lo = t0, hi = t1, alo = a0
      while (hi - lo > 1000) {
        const mid = lo + (hi - lo) / 2
        const am = f(mid) - level
        if ((am < 0) === (alo < 0)) { lo = mid; alo = am } else hi = mid
      }
      out.push({ t: Math.floor((lo + (hi - lo) / 2) / 1000) * 1000, rising: a0 < 0 })
    }
    t0 = t1; a0 = a1
  }
  return out
}

/** When a body is highest between two moments, and how high. */
export function culmination(f: (t: number) => number, from: number, to: number, step = 600000): [number, number] {
  let best = from, bestA = f(from)
  for (let t = from; t <= to; t += step) { const a = f(t); if (a > bestA) { best = t; bestA = a } }
  const a = f(to); if (a > bestA) { best = to; bestA = a }
  let lo = Math.max(from, best - step), hi = Math.min(to, best + step)
  while (hi - lo > 1000) {
    const m1 = lo + (hi - lo) / 3, m2 = lo + (hi - lo) * 2 / 3
    if (f(m1) < f(m2)) lo = m1; else hi = m2
  }
  const t = Math.floor((lo + (hi - lo) / 2) / 1000) * 1000
  const at2 = f(t)
  return at2 >= bestA ? [t, at2] : [best, bestA]
}

export const bodyAlt = (b: BodyKey, o: Observer) => (t: number) => at(b, o, t).alt
/** Altitude of a fixed point of the sky (J2000), precessed once for the moment. */
export function fixedAlt(ra2000: number, dec2000: number, o: Observer, near: number) {
  const [ra, dec] = radec(apply(precession(near), vec(ra2000, dec2000)))
  return (t: number) => horizontal(ra, dec, o, t)[0]
}

export function moonPhaseName(age: number) {
  age = rev(age)
  if (age < 10 || age >= 350) return 'New Moon'
  if (age < 80) return 'Waxing crescent'
  if (age < 100) return 'First quarter'
  if (age < 170) return 'Waxing gibbous'
  if (age < 190) return 'Full Moon'
  if (age < 260) return 'Waning gibbous'
  if (age < 280) return 'Last quarter'
  return 'Waning crescent'
}

/** The Moon's age in degrees past new (0 new, 180 full). */
export const moonAge = (t: number) => rev(body('moon', t).lon - body('sun', t).lon)

const COMPASS = ['north', 'north-east', 'east', 'south-east', 'south', 'south-west', 'west', 'north-west']
export const direction = (az: number) => COMPASS[Math.round(rev(az) / 45) % 8]
const SHORT = ['N', 'NE', 'E', 'SE', 'S', 'SW', 'W', 'NW']
export const dirShort = (az: number) => SHORT[Math.round(rev(az) / 45) % 8]
