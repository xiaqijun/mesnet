import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { useState, useEffect } from 'react'
import { useAuth } from '../context/AuthContext'
import { api } from '../api'

const nav = [
  { to: '/', label: '仪表盘' },
  { to: '/servers', label: '节点' },
  { to: '/tunnels', label: '隧道' },
  { to: '/topology', label: '拓扑' },
  { to: '/monitor', label: '流量监控' },
]

export default function Layout() {
  const { logout, user } = useAuth()
  const navigate = useNavigate()
  const [ver, setVer] = useState('')

  useEffect(() => {
    api.req('/agents/versions').then(d => setVer(d.server_version || '')).catch(() => {})
  }, [])

  const handleLogout = () => {
    logout()
    navigate('/login')
  }

  return (
    <div className="flex h-screen">
      <aside className="w-56 bg-gray-900 border-r border-gray-800 flex flex-col">
        <div className="p-4 border-b border-gray-800">
          <h1 className="text-lg font-bold text-emerald-400">MeshNet</h1>
          <p className="text-xs text-gray-500 mt-0.5">隐蔽 Mesh 网络</p>
        </div>
        <nav className="flex-1 p-3 space-y-1">
          {nav.map((n) => (
            <NavLink
              key={n.to}
              to={n.to}
              end={n.to === '/'}
              className={({ isActive }) =>
                `block px-3 py-2 rounded text-sm transition-colors ${
                  isActive
                    ? 'bg-emerald-500/10 text-emerald-400 font-medium'
                    : 'text-gray-400 hover:text-gray-200 hover:bg-gray-800'
                }`
              }
            >
              {n.label}
            </NavLink>
          ))}
        </nav>
        <div className="p-3 border-t border-gray-800 space-y-2">
          <div className="text-xs text-gray-600">{user?.username}</div>
          <button
            onClick={handleLogout}
            className="w-full px-3 py-1.5 text-xs text-gray-400 hover:text-gray-200 hover:bg-gray-800 rounded transition-colors text-left"
          >
            退出登录
          </button>
          {ver && <div className="text-[10px] text-gray-600">{ver}</div>}
        </div>
      </aside>
      <main className="flex-1 overflow-auto p-6">
        <Outlet />
      </main>
    </div>
  )
}
