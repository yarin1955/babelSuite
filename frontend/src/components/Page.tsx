import { useEffect, useState } from 'react'

interface Props {
    title: string
    toolbar?: React.ReactNode
    children: React.ReactNode
}

export default function Page({ title, toolbar, children }: Props) {
    const [sidebarCollapsed, setSidebarCollapsed] = useState(false)

    useEffect(() => {
        const check = () => {
            const sidebar = document.querySelector('.sidebar')
            setSidebarCollapsed(sidebar?.classList.contains('sidebar--collapsed') ?? false)
        }
        const observer = new MutationObserver(check)
        const sidebar = document.querySelector('.sidebar')
        if (sidebar) observer.observe(sidebar, { attributes: true, attributeFilter: ['class'] })
        return () => observer.disconnect()
    }, [])

    return (
        <div className={`sb-page-wrapper${sidebarCollapsed ? ' sb-page-wrapper--collapsed' : ''}`}>
            <div className='page page--has-toolbar'>
                <div className='page__top-bar'>
                    <div className='page__top-bar__title'>{title}</div>
                    {toolbar && <div className='page__top-bar__tools'>{toolbar}</div>}
                </div>
                <div className='page__body'>{children}</div>
            </div>
        </div>
    )
}
