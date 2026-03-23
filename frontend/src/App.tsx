import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import Login    from './pages/Login'
import Signup   from './pages/Signup'
import Home     from './pages/Home'
import Runs     from './pages/Runs'
import Suites   from './pages/Suites'
import Profiles from './pages/Profiles'
import Settings from './pages/Settings'

function Guard({ children }: { children: React.ReactNode }) {
    return localStorage.getItem('token') ? <>{children}</> : <Navigate to='/login' replace />
}

export default function App() {
    return (
        <BrowserRouter>
            <Routes>
                <Route path='/login'  element={<Login />} />
                <Route path='/signup' element={<Signup />} />
                <Route path='/'        element={<Guard><Home /></Guard>} />
                <Route path='/runs'    element={<Guard><Runs /></Guard>} />
                <Route path='/suites'  element={<Guard><Suites /></Guard>} />
                <Route path='/profiles' element={<Guard><Profiles /></Guard>} />
                <Route path='/settings' element={<Guard><Settings /></Guard>} />
                <Route path='*' element={<Navigate to='/' replace />} />
            </Routes>
        </BrowserRouter>
    )
}
