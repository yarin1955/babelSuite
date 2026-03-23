import { FaLayerGroup } from 'react-icons/fa6'
import Layout from '../components/Layout'
import Page from '../components/Page'

export default function Suites() {
    return (
        <Layout>
            <Page title='Suites'>
                <div className='empty-state'>
                    <div className='empty-state__icon'><FaLayerGroup /></div>
                    <h4>No suites yet</h4>
                    <p>Create a suite to define your pipeline topology.</p>
                </div>
            </Page>
        </Layout>
    )
}
