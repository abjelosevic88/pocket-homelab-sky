// Pocket Sky works without signal at a dark site: the app, the star
// catalogue and the last answers of the server are kept on the device.
// Pages and assets: the network first for the page, the cache first for the
// fingerprinted files; the server's answers (GET only): the network first,
// the last good copy when there is no connection.
const SHELL = 'psky-shell-v1'
const DATA = 'psky-data-v1'
const KEEP = /^\/api\/v1\/(catalog|tonight|nights|overview|summary|settings|auth\/status|auth\/me|events|bodies|minor|planets\/detail|moon|doubles|satellites\/passes|objects\/|progress|targets|observations|plans|kit|locations|equipment|darkness)/

self.addEventListener('install', e => {
  e.waitUntil(caches.open(SHELL).then(c => c.addAll(['/', '/favicon.svg', '/manifest.webmanifest'])).then(() => self.skipWaiting()))
})

self.addEventListener('activate', e => {
  e.waitUntil(caches.keys().then(ks => Promise.all(ks.filter(k => k !== SHELL && k !== DATA).map(k => caches.delete(k)))).then(() => self.clients.claim()))
})

self.addEventListener('message', e => {
  // signed out: forget the person's data on this device
  if (e.data === 'forget') e.waitUntil(caches.delete(DATA))
})

self.addEventListener('fetch', e => {
  const req = e.request
  if (req.method !== 'GET') return
  const url = new URL(req.url)
  if (url.origin !== location.origin) return
  if (url.pathname.startsWith('/assets/') || /\.(svg|png|webmanifest)$/.test(url.pathname)) {
    e.respondWith(caches.match(req).then(hit => hit || fetch(req).then(res => {
      if (res.ok) { const copy = res.clone(); caches.open(SHELL).then(c => c.put(req, copy)) }
      return res
    })))
    return
  }
  if (url.pathname.startsWith('/api/')) {
    if (!KEEP.test(url.pathname)) return
    e.respondWith(fetch(req).then(res => {
      if (res.ok) { const copy = res.clone(); caches.open(DATA).then(c => c.put(req, copy)) }
      return res
    }).catch(() => caches.match(req).then(hit => {
      if (hit) {
        const h = new Headers(hit.headers)
        h.set('X-Pocket-Offline', '1')
        return hit.blob().then(b => new Response(b, { status: 200, headers: h }))
      }
      return new Response(JSON.stringify({ error: 'No connection, and nothing saved for this yet' }), { status: 503, headers: { 'Content-Type': 'application/json' } })
    })))
    return
  }
  if (req.mode === 'navigate') {
    e.respondWith(fetch(req).then(res => {
      if (res.ok) { const copy = res.clone(); caches.open(SHELL).then(c => c.put('/', copy)) }
      return res
    }).catch(() => caches.match('/')))
  }
})
