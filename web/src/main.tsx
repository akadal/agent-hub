import { createRoot } from 'react-dom/client'

import App from './App.tsx'
import './index.css'

// No StrictMode: it double-mounts effects in dev and tears down live
// WebSocket + remote SSH sessions mid-handshake (black xterm, "connected"
// with no prompt) — especially painful on slower Tailscale hosts.
createRoot(document.getElementById('root')!).render(<App />)
