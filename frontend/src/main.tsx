import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import App from './app/App'
import './styles/index.css'
import { useSettingsStore } from './state/useSettingsStore'

// Self-hosted type: Montserrat (UI/body) + Cascadia Mono (data/labels).
import '@fontsource/montserrat/400.css'
import '@fontsource/montserrat/500.css'
import '@fontsource/montserrat/600.css'
import '@fontsource/montserrat/700.css'
import '@fontsource/cascadia-mono/400.css'
import '@fontsource/cascadia-mono/500.css'
import '@fontsource/cascadia-mono/600.css'

useSettingsStore.getState().init()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>,
)