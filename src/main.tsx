import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import './index.css'
import { AuthGate } from './components/AuthGate.tsx'
import { HexcraftApp } from './HexcraftApp.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <AuthGate>{() => <HexcraftApp />}</AuthGate>
    </BrowserRouter>
  </StrictMode>,
)

// Installable as an app; the service worker lets it open offline.
if (import.meta.env.PROD && 'serviceWorker' in navigator) {
  window.addEventListener('load', () => void navigator.serviceWorker.register('/sw.js'));
}
