import { Routes, Route, Navigate } from 'react-router-dom'
import { AuthProvider, useAuth } from './context/AuthContext'
import Layout from './components/Layout'
import Login from './pages/Login'
import ChangePassword from './pages/ChangePassword'
import Dashboard from './pages/Dashboard'
import Servers from './pages/Servers'
import NodeDetail from './pages/NodeDetail'
import Tunnels from './pages/Tunnels'
import TunnelDetail from './pages/TunnelDetail'
import Topology from './pages/Topology'
import Monitor from './pages/Monitor'
import Audit from './pages/Audit'

function RequireAuth({ children, requireChangePass }) {
  const { user, loading } = useAuth()

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-950 flex items-center justify-center">
        <p className="text-gray-500">加载中...</p>
      </div>
    )
  }

  if (!user) {
    return <Navigate to="/login" replace />
  }

  // Must change password — redirect unless already on change-password page
  if (requireChangePass && !user.must_change_pass) {
    return children
  }
  if (!requireChangePass && user.must_change_pass) {
    return <Navigate to="/change-password" replace />
  }

  return children
}

function AppRoutes() {
  const { user } = useAuth()

  return (
    <Routes>
      <Route
        path="/login"
        element={user ? <Navigate to="/" replace /> : <Login />}
      />
      <Route
        path="/change-password"
        element={
          <RequireAuth requireChangePass>
            <ChangePassword />
          </RequireAuth>
        }
      />
      <Route
        path="/"
        element={
          <RequireAuth>
            <Layout />
          </RequireAuth>
        }
      >
        <Route index element={<Dashboard />} />
        <Route path="servers" element={<Servers />} />
        <Route path="nodes/:id" element={<NodeDetail />} />
        <Route path="tunnels" element={<Tunnels />} />
        <Route path="tunnels/:id" element={<TunnelDetail />} />
        <Route path="topology" element={<Topology />} />
        <Route path="monitor" element={<Monitor />} />
        <Route path="audit" element={<Audit />} />
      </Route>
    </Routes>
  )
}

export default function App() {
  return (
    <AuthProvider>
      <AppRoutes />
    </AuthProvider>
  )
}
