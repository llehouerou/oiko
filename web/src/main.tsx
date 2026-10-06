import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import './index.css'
import { followHistory } from './history'
import { knowMe } from './access'

// Oiko has no service worker: one here is left by what this name served before
// (a Home Assistant's caches its pages and cuts the event stream), so it goes,
// and a page it controls loads again without it.
void navigator.serviceWorker?.getRegistrations().then(async (registrations) => {
  await Promise.all(registrations.map((r) => r.unregister()))
  if (navigator.serviceWorker.controller) location.reload()
})
followHistory()
void knowMe()
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
