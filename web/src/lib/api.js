// Thin wrappers over the Go JSON API. Every call surfaces the server's
// {error} message as a thrown Error so views can show it.

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

export const getFilters = () => req('/api/filters')
export const analyze = (url, profile) => post('/api/analyze', { url, profile })
export const discover = (payload) => post('/api/discover', payload)
export const simulate = (url, profile) => post('/api/simulate', { url, profile })
export const applicant = (name, score) => post('/api/applicant', { name, score })
export const predict = (urls, profile, excludeUnlikely = false) =>
  post('/api/predict', { urls, profile, excludeUnlikely })
