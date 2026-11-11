import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App.tsx'
import './fonts.css'
import './index.css'
import './manga.css'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter } from 'react-router'
import { AuthProvider } from './context/AuthContext.tsx'
import { ThemeProvider } from './context/ThemeContext.tsx'
import { NotificationProvider } from './context/NotificationContext.tsx'
import { RoleSyncWarning } from './components/RoleSyncWarning.tsx'
import ErrorBoundary from './components/ErrorBoundary.tsx'
import { reloadOnceForNewBuild } from './utils/chunkReload.ts'

window.addEventListener('vite:preloadError', (event) => {
  if (reloadOnceForNewBuild()) event.preventDefault()
})

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: false,
      refetchOnWindowFocus: false,
    },
  },
})

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
          <BrowserRouter>
            <AuthProvider>
              <ThemeProvider>
                <NotificationProvider>
                  <RoleSyncWarning />
                  <App />
                </NotificationProvider>
              </ThemeProvider>
            </AuthProvider>
          </BrowserRouter>
      </QueryClientProvider>
    </ErrorBoundary>
  </React.StrictMode>,
)
