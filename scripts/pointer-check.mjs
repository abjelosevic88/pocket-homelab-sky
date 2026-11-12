// "Point at the sky" on an iPhone, replayed: the gyro's relative angles plus a
// compass heading that flips round when the phone points up at the sky (as
// iOS does). The view must follow the true direction and never jump.
// Usage: node scripts/pointer-check.mjs
import { build } from '../web/node_modules/esbuild/lib/main.js'
import { mkdtempSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = join(dirname(fileURLToPath(import.meta.url)), '..')
const out = join(mkdtempSync(join(tmpdir(), 'ptr-')), 'p.mjs')
const handlers = {}
globalThis.window = { addEventListener: (n, f) => { (handlers[n] ||= []).push(f) }, removeEventListener() {} }
globalThis.screen = { orientation: { angle: 0 } }
await build({ entryPoints: [join(root, 'web/src/lib/orientation.ts')], bundle: true, format: 'esm', platform: 'node', outfile: out, logLevel: 'error' })
const { Pointer } = await import(out)

let frame = null
const p = new Pointer(f => { frame = f })
p.start()
let seed = 7
const noise = a => { seed = (seed * 16807) % 2147483647; return (seed / 2147483647 - 0.5) * 2 * a }
const wrap = d => ((d + 540) % 360) - 180
const send = (A, E, drift) => {
  const alpha = (360 - A + 360) % 360
  const flat = E < -55 // (not used here)
  let heading = E < 40 ? A : A + 180 // iOS: the camera when upright, flipped when pointing up
  if (flat) heading = A
  const ev = { alpha: (alpha + 37 + drift + 360) % 360, beta: 90 + E, gamma: 0, absolute: false, webkitCompassHeading: (heading + noise(4) + 360) % 360, webkitCompassAccuracy: 10 }
  for (const h of handlers.deviceorientation) h(ev)
  const f = frame.f
  return { az: (Math.atan2(f[0], f[1]) * 180 / Math.PI + 360) % 360, alt: Math.atan2(f[2], Math.hypot(f[0], f[1])) * 180 / Math.PI }
}
const path = []
for (let i = 0; i < 90; i++) path.push([180, 5])                     // held upright, facing south: north is found
for (let i = 0; i < 90; i++) path.push([180, 5 + (55 * i) / 89])      // raised to 60° up
for (let i = 0; i < 180; i++) path.push([180 + (90 * i) / 179, 60])   // panned west, high up (the compass is wrong now)
for (let i = 0; i < 90; i++) path.push([270, 60 - (50 * i) / 89])     // lowered again
let bad = 0, prev = null, worst = 0, jump = 0
path.forEach(([A, E], i) => {
  const r = send(A, E, i * 0.01)
  if (i > 60) {
    const off = Math.max(Math.abs(wrap(r.az - A)) * Math.cos(E * Math.PI / 180), Math.abs(r.alt - E))
    worst = Math.max(worst, off)
    if (off > 8) bad++
  }
  if (prev && i > 60) jump = Math.max(jump, Math.abs(wrap(r.az - prev.az)) * Math.cos(E * Math.PI / 180), Math.abs(r.alt - prev.alt))
  prev = r
})
console.log(`worst off ${worst.toFixed(1)}°, biggest step ${jump.toFixed(1)}°`)
if (bad || jump > 4) { console.log(`✗ the view jumps or is off (${bad} readings more than 8° off)`); process.exit(1) }
console.log('pointing follows the phone without jumping')
