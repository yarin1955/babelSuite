import { FaPlay } from 'react-icons/fa6'
import Layout from '../components/Layout'
import Page from '../components/Page'

export default function Runs() {
    return (
        <Layout>
            <Page title='Runs'>
                <div className='empty-state'>
                    <div className='empty-state__icon'><FaPlay /></div>
                    <h4>No runs yet</h4>
                    <p>Trigger a suite run to see results here.</p>
                </div>
            </Page>
        </Layout>
    )
}
