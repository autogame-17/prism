import React from 'react'
import ReactDOM from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from './App'
import { Toaster } from './components/ui/toaster'
import { applyStoredTheme } from './lib/theme'
import './styles.css'

// Apply the persisted theme synchronously, before React renders, so the
// first paint already matches the user's saved preference. Otherwise the
// hard-coded `<html class="dark">` in index.html would flash and snap once
// SettingsPage mounted later.
applyStoredTheme()

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false } },
})

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
      <Toaster />
    </QueryClientProvider>
  </React.StrictMode>
)
