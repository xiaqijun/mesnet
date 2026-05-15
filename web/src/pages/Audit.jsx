import { usePolling } from '../hooks/usePolling'

export default function Audit() {
  const { data, loading } = usePolling(() =>
    fetch('/api/logs').then(r => r.json()), 3000
  )

  if (loading) return <div className="text-gray-500 text-sm p-6">加载中...</div>

  const logs = data?.logs || []
  const errors = data?.errors || []

  return (
    <div>
      <h2 className="text-xl font-bold mb-2">日志审计</h2>
      <p className="text-[10px] text-gray-500 mb-4">
        错误 {data?.error_count || 0} · 最近 200 条
      </p>

      {errors.length > 0 && (
        <div className="mb-6">
          <h3 className="text-sm font-bold text-red-400 mb-2">错误</h3>
          <div className="space-y-1">
            {errors.map((e, i) => (
              <div key={i} className="bg-red-900/20 border border-red-800/30 rounded p-2 text-[11px]">
                <span className="text-red-400">{e.time?.slice(11, 19)}</span>
                <span className="text-gray-500 ml-2">[{e.source}]</span>
                <span className="text-red-300 ml-2">{e.message}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      <h3 className="text-sm font-bold text-gray-400 mb-2">全部</h3>
      <div className="space-y-1">
        {logs.map((e, i) => (
          <div key={i} className="bg-gray-900 border border-gray-800 rounded p-2 text-[11px] flex gap-2">
            <span className="text-gray-600 w-16">{e.time?.slice(11, 19)}</span>
            <span className={`w-12 ${e.level === 'ERROR' ? 'text-red-400' : e.level === 'WARN' ? 'text-amber-400' : 'text-emerald-400'}`}>{e.level}</span>
            <span className="text-gray-500 w-16">[{e.source}]</span>
            <span className="text-gray-300 truncate">{e.message}</span>
          </div>
        ))}
        {logs.length === 0 && <p className="text-xs text-gray-500 py-4 text-center">暂无日志</p>}
      </div>
    </div>
  )
}
