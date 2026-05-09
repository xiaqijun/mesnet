import { usePolling } from '../hooks/usePolling'
import { api } from '../api'

export default function Audit() {
  const { data, loading } = usePolling(() => api.listAudit().catch(() => ({ audit_logs: [] })), 5000)

  if (loading) return <div className="text-gray-500">加载中...</div>
  const logs = data?.audit_logs || []

  return (
    <div>
      <h2 className="text-xl font-bold mb-6">审计日志</h2>
      <div className="space-y-1">
        {logs.map((log) => (
          <div key={log.id} className="flex items-center gap-4 bg-gray-900 border border-gray-800 rounded-lg px-4 py-2.5">
            <span className={`text-[10px] px-1.5 py-0.5 rounded font-medium ${
              log.action.includes('create') ? 'bg-emerald-500/10 text-emerald-400' :
              log.action.includes('delete') ? 'bg-red-500/10 text-red-400' :
              log.action.includes('up') ? 'bg-blue-500/10 text-blue-400' :
              'bg-gray-700 text-gray-400'
            }`}>
              {log.action}
            </span>
            <span className="text-xs text-gray-300 flex-1">{log.detail}</span>
            <span className="text-[10px] text-gray-600">{new Date(log.created_at).toLocaleString()}</span>
          </div>
        ))}
        {logs.length === 0 && <p className="text-xs text-gray-500 py-8 text-center">暂无日志</p>}
      </div>
    </div>
  )
}
