import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import Login  from './pages/Login'
import Signup from './pages/Signup'
import Home   from './pages/Home'

function PrivateRoute({ children }: { children: React.ReactNode }) {
  return localStorage.getItem('token') ? <>{children}</> : <Navigate to='/login' replace />
}

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path='/login'  element={<Login />} />
        <Route path='/signup' element={<Signup />} />
        <Route path='/' element={<PrivateRoute><Home /></PrivateRoute>} />
        <Route path='*' element={<Navigate to='/' replace />} />
      </Routes>
    </BrowserRouter>
  )
}
