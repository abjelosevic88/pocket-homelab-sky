// Left column: log a night, the sections, your places with how tonight
// looks at each, and the next sky event — the Pocket Calendar sidebar.
import { Link, NavLink, useNavigate } from 'react-router-dom'
import { useApp } from '../lib/app-context'
import { useData } from '../lib/data'
import { fmtAt, fmtDayAt } from '../lib/format'
import { choosePlace } from '../lib/place'
import { Icon } from './icons'
import { EventIcon } from './SkyBits'

export function Sidebar({ onAdd, onNavigate }: { onAdd: () => void; onNavigate: () => void }) {
  const { status, settings } = useApp()
  const { overview } = useData()
  const navigate = useNavigate()
  const title = settings?.title || status?.title || 'Pocket Sky'
  const nav: { to: string; label: string; icon: string; end?: boolean }[] = [
    { to: '/', label: 'Tonight', icon: 'moon', end: true },
    { to: '/map', label: 'Sky map', icon: 'map' },
    { to: '/events', label: 'Sky events', icon: 'calendar' },
    { to: '/log', label: 'Observing log', icon: 'journal' },
    { to: '/lists', label: 'Lists', icon: 'target' },
    { to: '/places', label: 'Places', icon: 'pin' },
  ]
  const places = overview?.places || []
  const home = places.find(p => p.is_home)
  const next = overview?.next_event
  return (
    <aside className="sidebar" aria-label="Pocket Sky">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket Sky' ? title : <>Pocket <b>Sky</b></>}</span>
        {status?.version && <span className="sb-version" title={`Version ${status.version}`}>{/^v?\d+\.\d+/.test(status.version) ? 'v' + status.version.replace(/^v/, '').replace(/[-+].*$/, '') : status.version}</span>}
      </div>
      <button type="button" className="btn primary create-btn" onClick={onAdd}>
        <Icon name="plus" size={15} strokeWidth={2.2} />Log a night
      </button>

      <nav className="sb-nav" aria-label="Sections">
        {nav.map(n => (
          <NavLink key={n.to} to={n.to} end={n.end} onClick={onNavigate} className={({ isActive }) => `sb-nav-item ${isActive ? 'active' : ''}`}>
            <Icon name={n.icon} size={16} />{n.label}
          </NavLink>
        ))}
      </nav>

      {places.length > 0 && <>
        <div className="sb-section">Tonight at your places</div>
        <nav className="sb-nav sb-places" aria-label="Your places">
          {places.map(p => (
            <button key={p.id} type="button" className="sb-nav-item" onClick={() => { choosePlace(p.id); onNavigate(); navigate('/') }}>
              <Icon name={p.is_home ? 'home' : 'pin'} size={15} />
              <span className="ellipsis grow">{p.name}</span>
              {p.tonight && <span className={`sb-score tone-${p.tonight.score.label.toLowerCase()}`} title={p.tonight.score.words}>{p.tonight.score.forecast ? p.tonight.score.label : '—'}</span>}
            </button>
          ))}
        </nav>
      </>}

      {next && <>
        <div className="sb-section">Next in the sky</div>
        <Link to={`/events#${encodeURIComponent(next.key)}`} className="sb-next" onClick={onNavigate}>
          <EventIcon kind={next.kind} sub={next.sub} />
          <span className="grow" style={{ minWidth: 0 }}><span className="ellipsis block-line">{next.title}</span><span className="muted small">{fmtDayAt(next.t, home?.timezone)} {fmtAt(next.t, home?.timezone)}</span></span>
        </Link>
      </>}

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); window.dispatchEvent(new CustomEvent('psky:shortcuts')) }}><Icon name="keyboard" size={15} />Shortcuts</button>
      </div>
    </aside>
  )
}
