import { useNavigate } from 'react-router-dom'
import Layout from '../components/Layout'
import Page from '../components/Page'

export default function Settings() {
    const nav = useNavigate()
    const raw  = localStorage.getItem('user')
    const user = raw ? JSON.parse(raw) : null

    const logout = () => { localStorage.clear(); nav('/login') }

    return (
        <Layout>
            <Page title='Settings'>
                <div className='white-box' style={{ maxWidth: 480 }}>
                    <div className='white-box__details'>
                        <div className='white-box__details-row'>
                            <span className='white-box__details-row__name'>Name</span>
                            <span className='white-box__details-row__value'>{user?.name}</span>
                        </div>
                        <div className='white-box__details-row'>
                            <span className='white-box__details-row__name'>Username</span>
                            <span className='white-box__details-row__value'>@{user?.username}</span>
                        </div>
                        <div className='white-box__details-row'>
                            <span className='white-box__details-row__name'>Email</span>
                            <span className='white-box__details-row__value'>{user?.email}</span>
                        </div>
                    </div>
                </div>
                <button
                    onClick={logout}
                    style={{
                        background: 'none', border: '1.5px solid #e96d76', borderRadius: 6,
                        color: '#e96d76', padding: '8px 20px', fontSize: 14,
                        fontWeight: 500, cursor: 'pointer',
                    }}
                >
                    Sign out
                </button>
            </Page>
        </Layout>
    )
}
