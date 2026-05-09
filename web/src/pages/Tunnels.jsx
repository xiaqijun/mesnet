import { Link } from 'react-router-dom'
import { usePolling } from '../hooks/usePolling'
import { api } from '../api'
import StatusBadge from '../components/StatusBadge'

export default function Tunnels() {
  const { data, loading } = usePolling(() => api.listTunnels(), 3000)

  const handleUp = async (id) => { await api.tunnelUp(id) }
  const handleDown = async (id) => { await api.tunnelDown(id) }
  const handleDelete = async (id) => {
    if (!confirm('确定删除此隧道？')) return
    await api.deleteTunnel(id)
  }

  if (loading) return <div className="text-gray-500">加载中...</div>
  const tunnels = data?.tunnels || []

  function fmt(n) {
    if (!n) return '0 B'
    if (n >= 1e6) return (n / 1e6).toFixed(1) + ' MB'
    if (n >= 1e3) return (n / 1e3).toFixed(1) + ' KB'
    return n + ' B'
  }

  return (
    <div>
      <h2 className="text-xl font-bold mb-6">隧道管理</h2>

      <div className="space-y-2">
        {tunnels.map((t) => (
          <div key={t.id} className="bg-gray-900 border border-gray-800 rounded-lg p-4">
            <div className="flex items-center justify-between">
              <div className="flex-1">
                <div className="flex items-center gap-3 mb-2">
                  <Link to={`/tunnels/${t.id}`} className="text-sm font-medium text-emerald-400 hover:underline">{t.name}</Link>
                  <StatusBadge online={t.status === 'up'} label={t.status === 'up' ? 'UP' : 'DOWN'} />
                </div>
                <div className="flex gap-6 text-[10px] text-gray-500">
                  <span>{t.left_node?.name || `Node#${t.left_node_id}`} → {t.right_node?.name || `Node#${t.right_node_id}`}</span>
                  <span className="text-emerald-500">RX {fmt(t.rx_bytes)}</span>
                  <span className="text-blue-500">TX {fmt(t.tx_bytes)}</span>
                  {t.latency_ms > 0 && <span>延迟 {t.latency_ms.toFixed(1)}ms</span>}
                </div>
              </div>
              <div className="flex gap-2">
                {t.status !== 'up' && (
                  <button onClick={() => handleUp(t.id)} className="px-2 py-1 text-[10px] bg-emerald-600/20 text-emerald-400 rounded hover:bg-emerald-600/40">启动</button>
                )}
                {t.status === 'up' && (
                  <button onClick={() => handleDown(t.id)} className="px-2 py-1 text-[10px] bg-amber-600/20 text-amber-400 rounded hover:bg-amber-600/40">停止</button>
                )}
                <button onClick={() => handleDelete(t.id)} className="px-2 py-1 text-[10px] bg-red-600/20 text-red-400 rounded hover:bg-red-600/40">删除</button>
              </div>
            </div>
          </div>
        ))}
        {tunnels.length === 0 && <p className="text-center text-gray-500 py-8">暂无隧道</p>}
      </div>
    </div>
  )
}
