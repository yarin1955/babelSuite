import { useLocation, useNavigate } from 'react-router-dom'
import {
    FaPlay, FaLayerGroup, FaSliders, FaGear,
    FaChevronLeft, FaChevronRight,
} from 'react-icons/fa6'
import { useState } from 'react'

const NAV = [
    { path: '/runs',     icon: <FaPlay />,       label: 'Runs'     },
    { path: '/suites',   icon: <FaLayerGroup />, label: 'Suites'   },
    { path: '/profiles', icon: <FaSliders />,   label: 'Profiles' },
    { path: '/settings', icon: <FaGear />,        label: 'Settings' },
]

export default function Sidebar() {
    const [collapsed, setCollapsed] = useState(false)
    const location = useLocation()
    const nav = useNavigate()

    return (
        <div className={`sidebar${collapsed ? ' sidebar--collapsed' : ''}`}>
            <div className='sidebar__container'>
                <div className='sidebar__logo'>
                    <div className='sidebar__collapse-button' onClick={() => setCollapsed(c => !c)}>
                        {collapsed ? <FaChevronRight /> : <FaChevronLeft />}
                    </div>
                    {!collapsed && (
                        <div className='sidebar__logo-container' onClick={() => nav('/')}>
                            <div className='sidebar__logo__name'>BabelSuite</div>
                            <div className='sidebar__version'>v0.1.0</div>
                        </div>
                    )}
                    <div className='sidebar__logo__icon' onClick={() => nav('/')}>B</div>
                </div>

                {NAV.map(item => {
                    const active = location.pathname === item.path || location.pathname.startsWith(item.path + '/')
                    return (
                        <div
                            key={item.path}
                            className={`sidebar__nav-item${active ? ' sidebar__nav-item--active' : ''}`}
                            onClick={() => nav(item.path)}
                        >
                            <span className='nav-icon'>{item.icon}</span>
                            {!collapsed && item.label}
                        </div>
                    )
                })}
            </div>
        </div>
    )
}
