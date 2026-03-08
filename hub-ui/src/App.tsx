import { useEffect, useState } from 'react';

type Package = {
    name: string;
    version: string;
    description: string;
    provider: string;
    downloads: number;
};

function App() {
    const [packages, setPackages] = useState<Package[]>([]);
    const [search, setSearch] = useState('');
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        fetch('/api/v1/packages')
            .then(res => res.json())
            .then(data => {
                setPackages(data.packages || []);
                setLoading(false);
            })
            .catch(err => {
                console.error('Failed to fetch packages', err);
                setLoading(false);
            });
    }, []);

    const filtered = packages.filter(p => p.name.toLowerCase().includes(search.toLowerCase()) || p.description.toLowerCase().includes(search.toLowerCase()));

    return (
        <div className="hub-container">
            <header className="header">
                <div className="logo">BabelSuite Hub</div>
                <div className="search-bar">
                    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#94a3b8" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                        <circle cx="11" cy="11" r="8"></circle>
                        <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
                    </svg>
                    <input
                        type="text"
                        className="search-input"
                        placeholder="Search simulators, test suites..."
                        value={search}
                        onChange={e => setSearch(e.target.value)}
                    />
                </div>
                <nav style={{ display: 'flex', gap: '1.5rem', alignItems: 'center' }}>
                    <a href="#">Explore</a>
                    <a href="#">Documentation</a>
                    <a href="#" style={{ background: '#3b82f6', color: 'white', padding: '0.5rem 1rem', borderRadius: '4px', fontWeight: 600 }}>Sign In</a>
                </nav>
            </header>

            <div className="hero">
                <h1 className="hero-title">Discover Simulation Environments</h1>
                <p className="hero-subtitle">Find, install, and share test suites and containerized simulators built for BabelSuite.</p>
            </div>

            <main>
                {loading ? (
                    <div className="spinner"></div>
                ) : (
                    <div className="grid">
                        {filtered.map(pkg => (
                            <div key={pkg.name} className="card">
                                <div className="card-header">
                                    <h3 className="card-title">{pkg.name}</h3>
                                    <span className="card-version">{pkg.version}</span>
                                </div>
                                <div className="provider">
                                    <span className="badge">{pkg.provider}</span>
                                </div>
                                <p className="card-desc" style={{ marginTop: '1rem' }}>{pkg.description}</p>
                                <div className="card-footer">
                                    <span>{pkg.downloads.toLocaleString()} pulls</span>
                                    <a href="#">View config &rarr;</a>
                                </div>
                            </div>
                        ))}
                    </div>
                )}
            </main>
        </div>
    );
}

export default App;
