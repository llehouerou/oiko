import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import './index.css'
import { connect } from './store'
import { followHistory } from './history'
import { loadMe } from './access'

followHistory()
connect()
void loadMe()
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
