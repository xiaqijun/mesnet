import { usePolling } from '../hooks/usePolling'
import { api } from '../api'
import TrafficChart from '../components/TrafficChart'

export default function Monitor() {
  const { data, loading } = usePolling(() => api.getTotalTraffic(), 3000)

  if (loading) return <div className="text-gray-500">加载中...</div>
  const d = data || {}

  function fmt(n) {
    if (!n) return '0 B'
    if (n >= 1e9) return (n / 1e9).toFixed(2) + ' GB'
    if (n >= 1e6) return (n / 1e6).toFixed(1) + ' MB'
    if (n >= 1e3) return (n / 1e3).toFixed(1) + ' KB'
    return n + ' B'
  }

  const top = d.top_tunnels || []

  return (
    <div>
      <h2 className="text-xl font-bold mb-6">流量监控</h2>

      <div className="grid grid-cols-3 gap-4 mb-8">
        <div className="bg-gray-900 border border-gray-800 rounded-lg p-4">
          <p className="text-2xl font-bold text-emerald-400">{fmt(d.total_rx)}</p>
          <p className="text-xs text-gray-500 mt-1">全网总接收</p>
        </div>
        <div className="bg-gray-900 border border-gray-800 rounded-lg p-4">
          <p className="text-2xl font-bold text-blue-400">{fmt(d.total_tx)}</p>
          <p className="text-xs text-gray-500 mt-1">全网总发送</p>
        </div>
        <div className="bg-gray-900 border border-gray-800 rounded-lg p-4">
          <p className="text-2xl font-bold text-amber-400">{fmt(d.total_rx + d.total_tx)}</p>
          <p className="text-xs text-gray-500 mt-1">全网总流量</p>
        </div>
      </div>

      <h3 className="text-sm font-bold text-gray-300 mb-3">Top 隧道 (流量排行)</h3>
      <div className="space-y-2">
        {top.map((t) => (
          <div key={t.id} className="bg-gray-900 border border-gray-800 rounded-lg p-3">
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs font-medium text-gray-200">{t.name}</span>
              <span className="text-[10px] text-gray-500">{t.left_node?.name || ''} ↔ {t.right_node?.name || ''}</span>
            </div>
            <div className="flex items-center gap-3">
              <div className="flex-1 h-1.5 bg-gray-800 rounded-full overflow-hidden">
                <div
                  className="h-full bg-emerald-500 rounded-full transition-all"
                  style={{ width: d.total_rx + d.total_tx > 0 ? Math.min(100, ((t.rx_bytes + t.tx_bytes) / (d.total_rx + d.total_tx || 1)) * 100) + '%' : '0%' }}
                />
              </div>
              <span className="text-[10px] text-gray-500 w-20 text-right">{fmt(t.rx_bytes + t.tx_bytes)}</span>
            </div>
          </div>
        ))}
        {top.length === 0 && <p className="text-xs text-gray-500 py-4 text-center">暂无数据</p>}
      </div>
    </div>
  )
}
