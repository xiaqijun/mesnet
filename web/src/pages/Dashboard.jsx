import { usePolling } from '../hooks/usePolling'
import { api } from '../api'

export default function Dashboard() {
  const { data, loading } = usePolling(() => api.getStats(), 3000)

  if (loading) return <div className="text-gray-500">加载中...</div>

  const stats = data || {}

  const cards = [
    { label: '节点总数', value: stats.nodes, color: 'text-blue-400' },
    { label: '在线 Agent', value: stats.online_agents, color: 'text-emerald-400' },
    { label: '隧道总数', value: stats.tunnels, color: 'text-amber-400' },
    { label: '在线隧道', value: stats.online_tunnels, color: 'text-purple-400' },
  ]

  function fmt(n) {
    if (!n) return '0 B'
    if (n >= 1e9) return (n / 1e9).toFixed(1) + ' GB'
    if (n >= 1e6) return (n / 1e6).toFixed(1) + ' MB'
    if (n >= 1e3) return (n / 1e3).toFixed(1) + ' KB'
    return n + ' B'
  }

  return (
    <div>
      <h2 className="text-xl font-bold mb-6">仪表盘</h2>
      <div className="grid grid-cols-4 gap-4 mb-8">
        {cards.map((c) => (
          <div key={c.label} className="bg-gray-900 border border-gray-800 rounded-lg p-4">
            <p className={`text-2xl font-bold ${c.color}`}>{c.value ?? '-'}</p>
            <p className="text-xs text-gray-500 mt-1">{c.label}</p>
          </div>
        ))}
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="bg-gray-900 border border-gray-800 rounded-lg p-4">
          <h3 className="text-sm font-medium text-gray-400 mb-3">总流量</h3>
          <div className="flex gap-8">
            <div>
              <p className="text-xs text-gray-500">接收</p>
              <p className="text-lg font-bold text-emerald-400">{fmt(stats.total_rx)}</p>
            </div>
            <div>
              <p className="text-xs text-gray-500">发送</p>
              <p className="text-lg font-bold text-blue-400">{fmt(stats.total_tx)}</p>
            </div>
          </div>
        </div>
        <div className="bg-gray-900 border border-gray-800 rounded-lg p-4">
          <h3 className="text-sm font-medium text-gray-400 mb-3">状态概览</h3>
          <div className="space-y-2 text-xs text-gray-400">
            <div className="flex justify-between">
              <span>Agent 在线率</span>
              <span className="text-emerald-400">{stats.nodes ? Math.round((stats.online_agents / stats.nodes) * 100) : 0}%</span>
            </div>
            <div className="flex justify-between">
              <span>隧道在线率</span>
              <span className="text-emerald-400">{stats.tunnels ? Math.round((stats.online_tunnels / stats.tunnels) * 100) : 0}%</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
