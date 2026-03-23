import Sidebar from './Sidebar'

export default function Layout({ children }: { children: React.ReactNode }) {
    return (
        <div className='cd-layout'>
            <Sidebar />
            <div className='cd-layout__content'>
                {children}
            </div>
        </div>
    )
}
