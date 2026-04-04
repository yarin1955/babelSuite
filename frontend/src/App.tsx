import type { ReactNode } from 'react'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { getSession } from './lib/api'
import AuthCallback from './pages/AuthCallback'
import Catalog from './pages/Catalog'
import ForgotPassword from './pages/ForgotPassword'
import Home from './pages/Home'
import LiveExecution from './pages/LiveExecution'
import Profiles from './pages/Profiles'
import Sandboxes from './pages/Sandboxes'
import Settings from './pages/Settings'
import General from './pages/settings/General'
import Agents from './pages/settings/Agents'
import Registries from './pages/settings/Registries'
import Secrets from './pages/settings/Secrets'
import SignIn from './pages/SignIn'
import SignUp from './pages/SignUp'
import Suites from './pages/Suites'

function Guard({ children }: { children: ReactNode }) {
  return getSession() ? <>{children}</> : <Navigate to='/sign-in' replace />
}

function AdminGuard({ children }: { children: ReactNode }) {
  const session = getSession()
  if (!session) {
    return <Navigate to='/sign-in' replace />
  }
  return session.user.isAdmin ? <>{children}</> : <Navigate to='/' replace />
}

function GuestOnly({ children }: { children: ReactNode }) {
  return getSession() ? <Navigate to='/' replace /> : <>{children}</>
}

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path='/' element={<Guard><Home /></Guard>} />
        <Route path='/catalog' element={<Guard><Catalog /></Guard>} />
        <Route path='/suites' element={<Guard><Suites /></Guard>} />
        <Route path='/suites/:suiteId' element={<Guard><Suites /></Guard>} />
        <Route path='/executions/:executionId' element={<Guard><LiveExecution /></Guard>} />
        <Route path='/profiles' element={<Guard><Profiles /></Guard>} />
        <Route path='/environments' element={<Guard><Sandboxes /></Guard>} />
        <Route path='/sandbox' element={<Navigate to='/environments' replace />} />
        <Route path='/sandboxes' element={<Navigate to='/environments' replace />} />
        <Route path='/settings' element={<AdminGuard><Settings /></AdminGuard>} />
        <Route path='/settings/general' element={<AdminGuard><General /></AdminGuard>} />
        <Route path='/settings/agents' element={<AdminGuard><Agents /></AdminGuard>} />
        <Route path='/settings/registries' element={<AdminGuard><Registries /></AdminGuard>} />
        <Route path='/settings/secrets' element={<AdminGuard><Secrets /></AdminGuard>} />
        <Route path='/auth/callback' element={<AuthCallback />} />
        <Route path='/sign-in' element={<GuestOnly><SignIn /></GuestOnly>} />
        <Route path='/sign-up' element={<GuestOnly><SignUp /></GuestOnly>} />
        <Route path='/forgot-password' element={<GuestOnly><ForgotPassword /></GuestOnly>} />
        <Route path='*' element={<Navigate to='/' replace />} />
      </Routes>
    </BrowserRouter>
  )
}
