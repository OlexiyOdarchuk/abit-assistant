// Thin wrappers over the AbitAssistant backend. The SAME frontend runs in two
// shells:
//   • web server — talks to the Go JSON API over HTTP (fetch /api/…).
//   • desktop app — talks to the Go core over Wails bindings (window.go.main.App).
// Both speak identical apidto shapes, so views never care which is live. When
// Wails is present we prefer its bindings (there's no HTTP server on the
// desktop); otherwise we fall back to fetch. Checked lazily so it works
// whenever the Wails runtime finishes injecting window.go.

// wailsApp returns the bound Go App when inside the desktop shell, else null.
function wailsApp() {
  if (typeof window === 'undefined') return null
  return (window.go && window.go.main && window.go.main.App) || null
}

async function req(path, opts) {
  const r = await fetch(path, opts)
  if (!r.ok) {
    let msg = r.statusText
    try {
      const body = await r.json()
      if (body && body.error) msg = body.error
    } catch (_) {}
    throw new Error(msg)
  }
  return r.json()
}

const post = (path, body) =>
  req(path, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(body),
  })

// Each call: use the Wails binding if available, else the HTTP endpoint. A
// rejected binding promise carries the Go error message, matching the HTTP
// path's thrown Error — so view-level catch blocks stay identical.
export const getFilters = () => {
  const app = wailsApp()
  return app ? app.GetFilters() : req('/api/filters')
}
export const analyze = (url, profile) => {
  const app = wailsApp()
  return app ? app.Analyze(url, profile) : post('/api/analyze', { url, profile })
}
export const discover = (payload) => {
  const app = wailsApp()
  return app ? app.Discover(payload) : post('/api/discover', payload)
}
export const simulate = (url, profile, deep = false) => {
  const app = wailsApp()
  return app ? app.Simulate(url, profile, deep) : post('/api/simulate', { url, profile, deep })
}
export const applicant = (name, score) => {
  const app = wailsApp()
  return app ? app.Applicant(name, score) : post('/api/applicant', { name, score })
}
export const predict = (urls, profile, excludeUnlikely = false) => {
  const app = wailsApp()
  return app ? app.Predict(urls, profile, excludeUnlikely) : post('/api/predict', { urls, profile, excludeUnlikely })
}
