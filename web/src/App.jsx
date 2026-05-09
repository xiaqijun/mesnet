import { Routes, Route } from 'react-router-dom'
import Layout from './components/Layout'
import Dashboard from './pages/Dashboard'
import Servers from './pages/Servers'
import Nodes from './pages/Nodes'
import NodeDetail from './pages/NodeDetail'
import Tunnels from './pages/Tunnels'
import TunnelDetail from './pages/TunnelDetail'
import Topology from './pages/Topology'
import Monitor from './pages/Monitor'
import Audit from './pages/Audit'

export default function App() {
  return (
    <Layout>
      <Routes>
        <Route path="/" element={<Dashboard />} />
        <Route path="/servers" element={<Servers />} />
        <Route path="/nodes" element={<Nodes />} />
        <Route path="/nodes/:id" element={<NodeDetail />} />
        <Route path="/tunnels" element={<Tunnels />} />
        <Route path="/tunnels/:id" element={<TunnelDetail />} />
        <Route path="/topology" element={<Topology />} />
        <Route path="/monitor" element={<Monitor />} />
        <Route path="/audit" element={<Audit />} />
      </Routes>
    </Layout>
  )
}
