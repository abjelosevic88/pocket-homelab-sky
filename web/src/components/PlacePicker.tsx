// The place a page is about: your places, or where you are now.
import { usePlace } from '../lib/place'
import { openAddPlace } from './Layout'
import { Select } from './Select'
import { useToast } from './ui'

export function PlacePicker({ className = '' }: { className?: string }) {
  const { place, places, setChoice, locate, here } = usePlace()
  const { toast } = useToast()
  const options = [
    ...places.map(p => ({ value: String(p.id), label: p.name, hint: p.is_home ? 'home' : p.bortle ? `dark ${10 - p.bortle}/9` : '' })),
    { value: 'here', label: 'Where I am now', hint: here ? 'located' : 'uses your location' },
    { value: 'add', label: 'Add a place…' },
  ]
  const value = place?.id ? String(place.id) : place ? 'here' : ''
  return (
    <Select className={`place-picker ${className}`} ariaLabel="Place" value={value} options={options} placeholder="Choose a place"
      onChange={v => {
        if (v === 'add') { openAddPlace(); return }
        if (v === 'here') { locate().then(p => { if (!p) toast('Your location could not be found', 'err') }); return }
        setChoice(Number(v))
      }} />
  )
}
