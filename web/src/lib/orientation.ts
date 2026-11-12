// "Point at the sky": which way the phone's back camera is facing, from the
// phone's compass and tilt sensors, as a view frame (east-north-up) for the
// map. Android gives the compass direction straight away
// (deviceorientationabsolute); iPhones give a heading to line the frame up
// with (webkitCompassHeading). Readings are smoothed so the stars don't
// shake, and a drag sideways corrects a compass that is a little off.
import type { Vec3 } from './astro'
import { cross, dot, norm } from './sky'

export interface Frame { f: Vec3; u: Vec3; r: Vec3 }

type OrientationEventIOS = DeviceOrientationEvent & { webkitCompassHeading?: number; webkitCompassAccuracy?: number }
type PermissionAPI = { requestPermission?: () => Promise<'granted' | 'denied'> }

export const sensorsAvailable = () => typeof window !== 'undefined' && 'DeviceOrientationEvent' in window

/** Asks for the motion sensors (an iPhone asks the person; call from a tap). */
export async function askSensors(): Promise<boolean> {
  const D = (window as unknown as { DeviceOrientationEvent?: PermissionAPI }).DeviceOrientationEvent
  if (D && typeof D.requestPermission === 'function') {
    try { return (await D.requestPermission()) === 'granted' } catch { return false }
  }
  return sensorsAvailable()
}

const RAD = Math.PI / 180

/** Device → east-north-up rotation from the W3C angles (alpha about z, beta about x, gamma about y). */
function matrix(alpha: number, beta: number, gamma: number) {
  const cX = Math.cos(beta * RAD), cY = Math.cos(gamma * RAD), cZ = Math.cos(alpha * RAD)
  const sX = Math.sin(beta * RAD), sY = Math.sin(gamma * RAD), sZ = Math.sin(alpha * RAD)
  return {
    X: [cZ * cY - sZ * sX * sY, cY * sZ + cZ * sX * sY, -cX * sY] as Vec3,   // the device's right edge
    Y: [-cX * sZ, cZ * cX, sX] as Vec3,                                       // the device's top edge
    Z: [cY * sZ * sX + cZ * sY, sZ * sY - cZ * cY * sX, cX * cY] as Vec3,     // out of the screen
  }
}

const azOf = (v: Vec3) => Math.atan2(v[0], v[1]) / RAD

function rotZ(v: Vec3, deg: number): Vec3 {
  const c = Math.cos(deg * RAD), s = Math.sin(deg * RAD)
  return [v[0] * c + v[1] * s, -v[0] * s + v[1] * c, v[2]]
}

function screenAngle(): number {
  const so = (screen as Screen & { orientation?: { angle: number } }).orientation
  if (so && typeof so.angle === 'number') return so.angle
  const wo = (window as unknown as { orientation?: number }).orientation
  return typeof wo === 'number' ? wo : 0
}

export class Pointer {
  private onFrame: (f: Frame) => void
  private absolute = false
  private sf: Vec3 | null = null
  private su: Vec3 | null = null
  private iosOffset: number | null = null // relative alpha → north
  /** the person's own correction, degrees (drag sideways) */
  offset = 0
  heard = false
  private h1 = (e: Event) => this.handle(e as OrientationEventIOS, true)
  private h2 = (e: Event) => this.handle(e as OrientationEventIOS, false)

  constructor(onFrame: (f: Frame) => void) { this.onFrame = onFrame }

  start() {
    window.addEventListener('deviceorientationabsolute', this.h1)
    window.addEventListener('deviceorientation', this.h2)
  }

  stop() {
    window.removeEventListener('deviceorientationabsolute', this.h1)
    window.removeEventListener('deviceorientation', this.h2)
  }

  private handle(e: OrientationEventIOS, abs: boolean) {
    if (e.alpha == null || e.beta == null || e.gamma == null) return
    if (abs) this.absolute = true
    else if (this.absolute) return // the absolute one is better; use only it
    const m = matrix(e.alpha, e.beta, e.gamma)
    const th = screenAngle() * RAD
    // what the screen shows as up and right, turned with the screen
    let up: Vec3 = [Math.cos(th) * m.Y[0] + Math.sin(th) * m.X[0], Math.cos(th) * m.Y[1] + Math.sin(th) * m.X[1], Math.cos(th) * m.Y[2] + Math.sin(th) * m.X[2]]
    let fwd: Vec3 = [-m.Z[0], -m.Z[1], -m.Z[2]]
    let north = 0
    if (!abs && !e.absolute) {
      // iPhone: line the relative frame up with the compass heading
      const hd = e.webkitCompassHeading
      if (typeof hd === 'number' && !isNaN(hd) && (e.webkitCompassAccuracy ?? 0) >= 0) {
        const flat = Math.abs(e.beta) < 45 && Math.abs(e.gamma) < 45
        const ref = flat ? m.Y : fwd // the top edge when flat, the camera when held up
        const target = hd - azOf(ref)
        if (this.iosOffset == null) this.iosOffset = target
        else {
          // circular smoothing
          const d = ((target - this.iosOffset + 540) % 360) - 180
          this.iosOffset += d * 0.08
        }
      }
      north = this.iosOffset ?? 0
    }
    const turn = north + this.offset
    up = rotZ(up, turn)
    fwd = rotZ(fwd, turn)
    // smooth: follow fast movements, calm the shaking
    if (!this.sf || !this.su) { this.sf = fwd; this.su = up } else {
      const a = 0.22
      this.sf = norm([this.sf[0] + (fwd[0] - this.sf[0]) * a, this.sf[1] + (fwd[1] - this.sf[1]) * a, this.sf[2] + (fwd[2] - this.sf[2]) * a])
      this.su = norm([this.su[0] + (up[0] - this.su[0]) * a, this.su[1] + (up[1] - this.su[1]) * a, this.su[2] + (up[2] - this.su[2]) * a])
    }
    const f = this.sf
    const d = dot(this.su, f)
    const u = norm([this.su[0] - d * f[0], this.su[1] - d * f[1], this.su[2] - d * f[2]])
    this.heard = true
    this.onFrame({ f, u, r: cross(f, u) })
  }
}

/** The phone's back camera, as a stream for a <video>. */
export async function openCamera(): Promise<MediaStream> {
  if (!navigator.mediaDevices?.getUserMedia) throw new Error(window.isSecureContext ? 'This browser has no camera access.' : 'The camera needs the secure (https) address of Pocket Sky.')
  return navigator.mediaDevices.getUserMedia({ audio: false, video: { facingMode: { ideal: 'environment' }, width: { ideal: 1920 }, height: { ideal: 1080 } } })
}
