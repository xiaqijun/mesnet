import { useParams, Link } from 'react-router-dom'
import { useState } from 'react'
import { usePolling } from '../hooks/usePolling'
import { api } from '../api'
import StatusBadge from '../components/StatusBadge'
import TrafficChart from '../components/TrafficChart'

export default function NodeDetail() {
  const { id } = useParams()
  const { data, loading } = usePolling(() => api.getNode(id), 3000)
  const { data: statsData } = usePolling(() => api.getNodeStats(id).catch(() => ({})), 5000)
  const [testResults, setTestResults] = useState({})
  const [testing, setTesting] = useState(null)

  if (loading) return <div className="text-gray-500">加载中...</div>
  const node = data?.node
  const tunnels = data?.tunnels || []

  if (!node) return <div className="text-gray-500">节点不存在</div>

  const handleTunnelTest = async (tunnelId, peerId) => {
    setTesting(tunnelId)
    try {
      const res = await fetch(`/api/nodes/${id}/tunnel-test`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ target_node_id: peerId }),
      })
      const json = await res.json()
      setTestResults(prev => ({ ...prev, [tunnelId]: { result: json.result, server: json.server, error: json.error } }))
    } catch {
      setTestResults(prev => ({ ...prev, [tunnelId]: { error: '请求失败' } }))
    }
    setTesting(null)
  }

  return (
    <div>
      <Link to="/nodes" className="text-xs text-gray-500 hover:text-gray-300">&larr; 返回节点列表</Link>
      <div className="flex items-center gap-3 mt-2 mb-6">
        <h2 className="text-xl font-bold">{node.name}</h2>
        <StatusBadge online={data.online} />
      </div>

      <div className="grid grid-cols-4 gap-4 mb-6">
        {[
          { label: '虚拟 IP', value: node.virtual_ip || '-' },
          { label: 'Agent 版本', value: node.agent_version || '-' },
          { label: 'CPU', value: node.cpu ? node.cpu + ' 核' : '-' },
          { label: '内存', value: node.memory_mb ? (node.memory_mb >= 1024 ? (node.memory_mb / 1024).toFixed(1) + ' GB' : node.memory_mb + ' MB') : '-' },
        ].map((c) => (
          <div key={c.label} className="bg-gray-900 border border-gray-800 rounded-lg p-3">
            <p className="text-sm font-bold text-gray-200">{c.value}</p>
            <p className="text-xs text-gray-500">{c.label}</p>
          </div>
        ))}
      </div>

      {statsData && (
        <div className="grid grid-cols-3 gap-4 mb-6">
          {[
            { label: 'RX', value: statsData.rx_bytes, color: 'text-emerald-400' },
            { label: 'TX', value: statsData.tx_bytes, color: 'text-blue-400' },
            { label: '隧道数', value: `${statsData.up_count || 0}/${statsData.tunnel_count || 0}` },
          ].map((c) => (
            <div key={c.label} className="bg-gray-900 border border-gray-800 rounded-lg p-3">
              <p className={`text-sm font-bold ${c.color || 'text-gray-200'}`}>
                {typeof c.value === 'number' ? (c.value >= 1e6 ? (c.value / 1e6).toFixed(1) + ' MB' : c.value + ' B') : c.value}
              </p>
              <p className="text-xs text-gray-500">{c.label}</p>
            </div>
          ))}
        </div>
      )}

      <h3 className="text-sm font-bold text-gray-300 mb-3">关联隧道</h3>
      <div className="space-y-2">
        {tunnels.map((t) => {
          const peerId = t.left_node_id === node.id ? t.right_node_id : t.left_node_id
          const tr = testResults[t.id]
          return (
          <div key={t.id} className="bg-gray-900 border border-gray-800 rounded-lg p-3">
            <div className="flex items-center justify-between">
              <Link to={`/tunnels/${t.id}`} className="flex-1 hover:border-gray-700 transition-colors">
                <div>
                  <p className="text-xs font-medium text-gray-200">{t.name}</p>
                  <p className="text-[10px] text-gray-500 mt-0.5">{t.left_subnet} ↔ {t.right_subnet}</p>
                </div>
              </Link>
              <div className="flex items-center gap-2">
                <StatusBadge online={t.status === 'up'} label={t.status === 'up' ? 'UP' : 'DOWN'} />
                <button
                  onClick={() => handleTunnelTest(t.id, peerId)}
                  disabled={testing === t.id}
                  className="px-2 py-1 text-[10px] bg-blue-600/20 text-blue-400 rounded hover:bg-blue-600/40 disabled:opacity-50"
                >
                  {testing === t.id ? '测试中...' : '连通测试'}
                </button>
              </div>
            </div>
            {tr && (
              <div className="mt-2 text-[10px] space-y-1">
                {tr.error && !tr.result && (
                  <div className="text-red-400">{tr.error}</div>
                )}
                {tr.server && (
                  <div className="text-gray-500 space-x-2">
                    <span>服务端: </span>
                    <span className={tr.server.src_has_public_key ? 'text-emerald-400' : 'text-red-400'}>
                      {tr.server.src_has_public_key ? '源公钥✓' : '源公钥✗'}
                    </span>
                    <span className={tr.server.dst_has_public_key ? 'text-emerald-400' : 'text-red-400'}>
                      {tr.server.dst_has_public_key ? '目标公钥✓' : '目标公钥✗'}
                    </span>
                    <span className={tr.server.src_online ? 'text-emerald-400' : 'text-red-400'}>
                      {tr.server.src_online ? '在线' : '离线'}
                    </span>
                  </div>
                )}
                {tr.result && (
                  <div className="space-x-3">
                    <span>信道: </span>
                    <span className={
                      tr.result.channel_status === 'established' ? 'text-emerald-400' :
                      tr.result.channel_status === 'establishing' ? 'text-amber-400' :
                      'text-red-400'
                    }>{tr.result.channel_status}</span>
                    <span className={tr.result.peer_connected ? 'text-emerald-400' : 'text-red-400'}>
                      peer: {tr.result.peer_connected ? '✓' : '✗'}
                    </span>
                    <span className={tr.result.has_peer_key ? 'text-emerald-400' : 'text-amber-400'}>
                      密钥: {tr.result.has_peer_key ? '✓' : '✗'}
                    </span>
                    <span className={tr.result.has_routes ? 'text-emerald-400' : 'text-amber-400'}>
                      路由: {tr.result.has_routes ? '✓' : '✗'}
                    </span>
                    {tr.result.rtt_ms > 0 && (
                      <span className={tr.result.rtt_ms < 50 ? 'text-emerald-400' : 'text-amber-400'}>
                        RTT: {tr.result.rtt_ms.toFixed(1)}ms
                      </span>
                    )}
                    <span className="text-gray-600">({tr.result.total_peers}peers {tr.result.total_routes}routes)</span>
                  </div>
                )}
              </div>
            )}
          </div>
        )})}
        {tunnels.length === 0 && <p className="text-xs text-gray-500">暂无隧道</p>}
      </div>
    </div>
  )
}
