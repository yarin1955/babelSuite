import { FaSliders } from 'react-icons/fa6'
import Layout from '../components/Layout'
import Page from '../components/Page'

export default function Profiles() {
    return (
        <Layout>
            <Page title='Profiles'>
                <div className='empty-state'>
                    <div className='empty-state__icon'><FaSliders /></div>
                    <h4>No profiles yet</h4>
                    <p>Profiles inject environment variables into your pipeline containers.</p>
                </div>
            </Page>
        </Layout>
    )
}
