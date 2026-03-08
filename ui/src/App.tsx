import { useEffect, useState } from 'react';
import ReactFlow, { Background, Controls } from 'reactflow';
import 'reactflow/dist/style.css';
import './index.css';

const App = () => {
    const [view, setView] = useState('pipeline'); // 'pipeline', 'simulators', 'tests', 'settings'
    const [nodes, setNodes] = useState([]);
    const [edges, setEdges] = useState([]);

    useEffect(() => {
        if (view === 'pipeline') {
            fetch('/api/dag')
                .then(res => res.json())
                .then(data => {
                    const positionedNodes = data.nodes.map((node: any, i: number) => ({
                        ...node,
                        position: { x: 250, y: 100 * (i + 1) }
                    }));
                    setNodes(positionedNodes);
                    setEdges(data.edges);
                })
                .catch(err => console.error("Error fetching DAG:", err));
        }
    }, [view]);

    return (
        <div className="layout">
            <aside className="sidebar">
                <div className="sidebar-header">
                    <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                        <polygon points="12 2 2 7 12 12 22 7 12 2"></polygon>
                        <polyline points="2 17 12 22 22 17"></polyline>
                        <polyline points="2 12 12 17 22 12"></polyline>
                    </svg>
                    BabelSuite Control
                </div>
                <nav className="sidebar-nav">
                    <div className={`nav-item ${view === 'pipeline' ? 'active' : ''}`} onClick={() => setView('pipeline')}>Execution DAG</div>
                    <div className={`nav-item ${view === 'simulators' ? 'active' : ''}`} onClick={() => setView('simulators')}>Simulators Catalog</div>
                    <div className={`nav-item ${view === 'tests' ? 'active' : ''}`} onClick={() => setView('tests')}>Test Suites</div>
                    <div className={`nav-item ${view === 'settings' ? 'active' : ''}`} onClick={() => setView('settings')}>Configuration</div>
                </nav>
            </aside>
            <main className="main-content">
                <div className="run-banner">
                    <div>
                        <span style={{ color: '#8b949e', marginRight: '10px' }}>Active Project:</span>
                        <span style={{ fontWeight: 600 }}>engine-hil / default</span>
                    </div>
                    <div>
                        <button className="btn">Deploy Pipeline</button>
                    </div>
                </div>

                <div className="workspace">
                    {view === 'pipeline' && (
                        <div style={{ height: 'calc(100vh - 270px)' }}>
                            <ReactFlow nodes={nodes} edges={edges} fitView>
                                <Background color="#30363d" gap={16} />
                                <Controls />
                            </ReactFlow>
                            <div className="console">
                                <div className="line info">[system] Docker engine hooked. Connected to TCP socket.</div>
                                <div className="line">[Daemon] Waiting for execution trigger...</div>
                                <div className="line success">[Engine] Network babelsuite-net-starlark created.</div>
                            </div>
                        </div>
                    )}

                    {view === 'simulators' && (
                        <div className="settings-panel">
                            <h2>Simulators Configuration</h2>
                            <p style={{ color: '#8b949e' }}>Manage which simulators this project runs locally.</p>
                            <div className="settings-group">
                                <h3>Mapped Simulators (env.yaml)</h3>
                                <ul>
                                    <li><b>engine-sim (v1.2.0)</b> - Hub origin</li>
                                </ul>
                                <button className="btn" style={{ marginTop: '15px', background: '#21262d', color: '#c9d1d9', border: '1px solid #30363d' }}>+ Add Simulator Binding</button>
                            </div>
                        </div>
                    )}

                    {view === 'tests' && (
                        <div className="settings-panel">
                            <h2>Test Suites</h2>
                            <p style={{ color: '#8b949e' }}>Manage your Starlark testing pipelines here.</p>
                            <div className="settings-group">
                                <h3>Configured Suites</h3>
                                <ul>
                                    <li><b>test-python (latest)</b> - Starlark DAG node mapping</li>
                                </ul>
                            </div>
                        </div>
                    )}

                    {view === 'settings' && (
                        <div className="settings-panel">
                            <h2>Engine Properties</h2>
                            <div className="settings-group">
                                <div className="form-group">
                                    <label className="form-label">Docker Socket Address</label>
                                    <input type="text" className="form-input" defaultValue="unix:///var/run/docker.sock" />
                                </div>
                                <div className="form-group">
                                    <label className="form-label">BabelSuite Hub Registry</label>
                                    <input type="text" className="form-input" defaultValue="https://hub.babelsuite.io" />
                                </div>
                                <button className="btn" style={{ marginTop: '15px' }}>Save Changes</button>
                            </div>
                        </div>
                    )}
                </div>
            </main>
        </div>
    );
};

export default App;
