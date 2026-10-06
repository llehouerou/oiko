import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import './index.css'
import { followHistory } from './history'
import { loadMe } from './access'

followHistory()
// Who is signed in decides what the page shows: asked until Oiko answers.
const knowMe = async () => (await loadMe()) || setTimeout(knowMe, 3000)
void knowMe()
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
