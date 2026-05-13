import { NavLink } from 'react-router-dom'
import { useState, useEffect } from 'react'

const nav = [
  { to: '/', label: '仪表盘' },
  { to: '/servers', label: '服务器' },
  { to: '/nodes', label: '节点' },
  { to: '/tunnels', label: '隧道' },
  { to: '/topology', label: '拓扑' },
  { to: '/monitor', label: '流量监控' },
]

export default function Layout({ children }) {
  const [ver, setVer] = useState('')
  useEffect(() => {
    fetch('/api/agents/versions').then(r => r.json()).then(d => setVer(d.server_version || ''))
  }, [])
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
        {ver && <div className="p-3 border-t border-gray-800 text-[10px] text-gray-600">{ver}</div>}
      </aside>
      <main className="flex-1 overflow-auto p-6">{children}</main>
    </div>
  )
}
