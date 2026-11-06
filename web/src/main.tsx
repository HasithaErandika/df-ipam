import { QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './app/App'
import { queryClient } from './app/queryClient'
import { ConfirmProvider, ErrorBoundary, Toaster } from './components/ui'
import './index.css'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}><ErrorBoundary><ConfirmProvider><App /></ConfirmProvider></ErrorBoundary><Toaster /></QueryClientProvider>
  </StrictMode>,
)
