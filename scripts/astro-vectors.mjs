// Runs the engine test vectors (internal/astro/testdata/vectors.json) against
// the browser engine (web/src/lib/astro.ts), so the Go server and the web
// map agree. Usage: node scripts/astro-vectors.mjs
import { build } from '../web/node_modules/esbuild/lib/main.js'
import { readFileSync, mkdtempSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = join(dirname(fileURLToPath(import.meta.url)), '..')
const out = join(mkdtempSync(join(tmpdir(), 'astro-')), 'astro.mjs')
await build({ entryPoints: [join(root, 'web/src/lib/astro.ts')], bundle: true, format: 'esm', platform: 'node', outfile: out, logLevel: 'error' })
const A = await import(out)
const vectors = JSON.parse(readFileSync(join(root, 'internal/astro/testdata/vectors.json'), 'utf8'))
let bad = 0
const near = (what, g, w, tol = 1e-7) => {
  if (w === undefined) return
  let d = g - w
  if (what.endsWith('ra') || what === 'az' || what === 'lst') d = ((d + 540) % 360) - 180
  if (!(Math.abs(d) <= tol)) { bad++; console.log(`✗ ${what}: got ${g} want ${w}`) }
}
for (const v of vectors) {
  const o = { lat: v.lat, lon: v.lon }
  if (v.body) {
    const s = A.at(v.body, o, v.t)
    near(v.body + ' ra', s.ra, v.ra); near(v.body + ' dec', s.dec, v.dec); near('alt', s.alt, v.alt); near('az', s.az, v.az)
    near(v.body + ' mag', s.mag, v.mag); near(v.body + ' phase', s.phase, v.phase); near(v.body + ' dist', s.dist, v.dist, 1e-9)
    continue
  }
  near('lst', A.siderealTime(v.t, v.lon), v.lst)
  const [ra, dec] = A.radec(A.apply(A.precession(v.t), A.vec(101.2875, -16.7161)))
  near('star ra', ra, v.star_ra); near('star dec', dec, v.star_dec)
  const sr = A.crossings(A.bodyAlt('sun', o), A.HORIZON_SUN, v.t, v.t + 36 * 3600e3).find(c => c.rising)
  const mr = A.crossings(A.bodyAlt('moon', o), A.HORIZON_SUN, v.t, v.t + 36 * 3600e3).find(c => c.rising)
  near('sunrise', sr ? sr.t : 0, v.sunrise, 2000); near('moonrise', mr ? mr.t : 0, v.moonrise, 2000)
}
writeFileSync(out, '')
if (bad) { console.log(`${bad} failed`); process.exit(1) }
console.log(`all ${vectors.length} engine vectors pass in the browser engine`)
