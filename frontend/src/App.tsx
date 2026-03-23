import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import Login        from './pages/Login'
import Signup       from './pages/Signup'
import SSOCallback  from './pages/SSOCallback'
import Home         from './pages/Home'
import Runs         from './pages/Runs'
import Suites       from './pages/Suites'
import Catalog      from './pages/Catalog'
import AdminCatalog from './pages/AdminCatalog'
import Profiles     from './pages/Profiles'
import Settings     from './pages/Settings'

function Guard({ children }: { children: React.ReactNode }) {
    return localStorage.getItem('token') ? <>{children}</> : <Navigate to='/login' replace />
}

function AdminGuard({ children }: { children: React.ReactNode }) {
    if (!localStorage.getItem('token')) return <Navigate to='/login' replace />
    try {
        const user = JSON.parse(localStorage.getItem('user') || '{}')
        if (!user.is_admin) return <Navigate to='/' replace />
    } catch { return <Navigate to='/' replace /> }
    return <>{children}</>
}

export default function App() {
    return (
        <BrowserRouter>
            <Routes>
                <Route path='/login'         element={<Login />} />
                <Route path='/signup'        element={<Signup />} />
                <Route path='/auth/callback' element={<SSOCallback />} />
                <Route path='/'              element={<Guard><Home /></Guard>} />
                <Route path='/runs'          element={<Guard><Runs /></Guard>} />
                <Route path='/suites'        element={<Guard><Suites /></Guard>} />
                <Route path='/catalog'       element={<Guard><Catalog /></Guard>} />
                <Route path='/admin/catalog' element={<AdminGuard><AdminCatalog /></AdminGuard>} />
                <Route path='/profiles'      element={<Guard><Profiles /></Guard>} />
                <Route path='/settings'      element={<Guard><Settings /></Guard>} />
                <Route path='*' element={<Navigate to='/' replace />} />
            </Routes>
        </BrowserRouter>
    )
}
