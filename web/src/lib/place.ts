// Which place the pages are about: one of the person's places (home by
// default) or "where I am now" from the browser's location. Kept per device.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useData } from './data'
import type { Place, PlaceRef } from './types'

const KEY = 'psky-place'
const HERE = 'psky-here'

export type Choice = number | 'here'

function readChoice(): Choice | null {
  const v = localStorage.getItem(KEY)
  if (!v) return null
  if (v === 'here') return 'here'
  const n = Number(v)
  return Number.isFinite(n) && n > 0 ? n : null
}

function readHere(): PlaceRef | null {
  try { const v = JSON.parse(sessionStorage.getItem(HERE) || localStorage.getItem(HERE) || 'null'); return v && typeof v.lat === 'number' ? v : null } catch { return null }
}

const tzHere = () => { try { return Intl.DateTimeFormat().resolvedOptions().timeZone } catch { return '' } }

/** Choose the place every page shows (any open page follows). */
export function choosePlace(c: Choice) {
  localStorage.setItem(KEY, String(c))
  window.dispatchEvent(new CustomEvent('psky:place', { detail: c }))
}

export function placeRef(p: Place): PlaceRef { return { id: p.id, name: p.name, lat: p.lat, lon: p.lon, timezone: p.timezone, bortle: p.bortle } }

/** The query for the API: location=ID or lat/lon/tz. */
export function placeQuery(p: PlaceRef | null): string {
  if (!p) return ''
  if (p.id) return `location=${p.id}`
  return `lat=${p.lat.toFixed(4)}&lon=${p.lon.toFixed(4)}&tz=${encodeURIComponent(p.timezone || '')}&name=${encodeURIComponent(p.name)}`
}

export function usePlace() {
  const { overview } = useData()
  const places = useMemo(() => overview?.places || [], [overview])
  const [choice, setChoiceRaw] = useState<Choice | null>(readChoice)
  const [here, setHere] = useState<PlaceRef | null>(readHere)
  const [locating, setLocating] = useState(false)
  const [error, setError] = useState('')
  const setChoice = useCallback((c: Choice) => { setChoiceRaw(c); choosePlace(c) }, [])
  useEffect(() => {
    const h = (e: Event) => { setChoiceRaw((e as CustomEvent<Choice>).detail); setHere(readHere()) }
    window.addEventListener('psky:place', h)
    return () => window.removeEventListener('psky:place', h)
  }, [])
  const locate = useCallback(() => new Promise<PlaceRef | null>(resolve => {
    if (!navigator.geolocation) { setError('This browser cannot tell where you are.'); resolve(null); return }
    setLocating(true); setError('')
    navigator.geolocation.getCurrentPosition(pos => {
      const p: PlaceRef = { name: 'Where you are', lat: pos.coords.latitude, lon: pos.coords.longitude, timezone: tzHere(), bortle: 0 }
      setHere(p); localStorage.setItem(HERE, JSON.stringify(p)); setLocating(false); setChoice('here'); resolve(p)
    }, err => {
      setLocating(false)
      setError(err.code === err.PERMISSION_DENIED ? 'Location is not allowed for this site.' : window.isSecureContext ? 'Your location could not be found.' : 'Your location needs the secure (https) address.')
      resolve(null)
    }, { enableHighAccuracy: false, timeout: 15000, maximumAge: 10 * 60 * 1000 })
  }), [setChoice])
  const place = useMemo<PlaceRef | null>(() => {
    if (choice === 'here' && here) return here
    const p = places.find(x => x.id === choice) || places.find(x => x.is_home) || places[0]
    if (p) return placeRef(p)
    return here
  }, [choice, here, places])
  // a removed place falls back to home
  useEffect(() => { if (typeof choice === 'number' && places.length && !places.some(p => p.id === choice)) setChoiceRaw(null) }, [choice, places])
  return { place, places, choice, setChoice, here, locate, locating, error, loaded: !!overview }
}
