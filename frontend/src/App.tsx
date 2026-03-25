import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import Login        from './pages/Login'
import Signup       from './pages/Signup'
import SSOCallback  from './pages/SSOCallback'
import Runs         from './pages/Runs'
import Catalog      from './pages/Catalog'
import AdminCatalog from './pages/AdminCatalog'
import Agents       from './pages/Agents'
import RunDetail    from './pages/RunDetail'
import Profiles     from './pages/Profiles'
import Settings     from './pages/Settings'

function Guard({ children }: { children: React.ReactNode }) {
    return localStorage.getItem('token') ? <>{children}</> : <Navigate to='/login' replace />
}

function AdminGuard({ children }: { children: React.ReactNode }) {
    if (!localStorage.getItem('token')) return <Navigate to='/login' replace />
    try {
        const user = JSON.parse(localStorage.getItem('user') || '{}')
        if (!user.is_admin) return <Navigate to='/runs' replace />
    } catch { return <Navigate to='/runs' replace /> }
    return <>{children}</>
}

export default function App() {
    return (
        <BrowserRouter>
            <Routes>
                <Route path='/login'         element={<Login />} />
                <Route path='/signup'        element={<Signup />} />
                <Route path='/auth/callback' element={<SSOCallback />} />
                <Route path='/'              element={<Navigate to='/runs' replace />} />
                <Route path='/runs'          element={<Guard><Runs /></Guard>} />
                <Route path='/runs/:id'      element={<Guard><RunDetail /></Guard>} />
                <Route path='/suites'        element={<Guard><Catalog /></Guard>} />
                <Route path='/profiles'      element={<Guard><Profiles /></Guard>} />
                <Route path='/settings'      element={<Guard><Settings /></Guard>} />
                <Route path='/settings/profiles' element={<Guard><Profiles /></Guard>} />
                <Route path='/settings/agents'   element={<AdminGuard><Agents /></AdminGuard>} />
                <Route path='/settings/catalog'  element={<AdminGuard><AdminCatalog /></AdminGuard>} />
                <Route path='/agents'        element={<AdminGuard><Navigate to='/settings/agents' replace /></AdminGuard>} />
                <Route path='/catalog'       element={<AdminGuard><Navigate to='/settings/catalog' replace /></AdminGuard>} />
                <Route path='/admin/catalog' element={<AdminGuard><Navigate to='/settings/catalog' replace /></AdminGuard>} />
                <Route path='*' element={<Navigate to='/runs' replace />} />
            </Routes>
        </BrowserRouter>
    )
}
