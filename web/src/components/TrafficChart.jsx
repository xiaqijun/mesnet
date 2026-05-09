import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts'

function fmt(n) {
  if (!n || n === 0) return '0'
  if (n >= 1e9) return (n / 1e9).toFixed(1) + ' GB'
  if (n >= 1e6) return (n / 1e6).toFixed(1) + ' MB'
  if (n >= 1e3) return (n / 1e3).toFixed(1) + ' KB'
  return n + ' B'
}

export default function TrafficChart({ data }) {
  if (!data || data.length === 0) {
    return <div className="text-gray-500 text-xs py-8 text-center">暂无流量数据</div>
  }

  const formatted = data.map((d) => ({ ...d, time: new Date(d.created_at).toLocaleTimeString() }))

  return (
    <ResponsiveContainer width="100%" height={200}>
      <LineChart data={formatted}>
        <CartesianGrid strokeDasharray="3 3" stroke="#1f2937" />
        <XAxis dataKey="time" stroke="#4b5563" fontSize={10} />
        <YAxis stroke="#4b5563" fontSize={10} tickFormatter={fmt} />
        <Tooltip
          contentStyle={{ background: '#111827', border: '1px solid #1f2937', borderRadius: 8, fontSize: 12 }}
          formatter={(v) => fmt(v)}
        />
        <Line type="monotone" dataKey="rx_bytes" stroke="#34d399" strokeWidth={2} dot={false} name="RX" />
        <Line type="monotone" dataKey="tx_bytes" stroke="#60a5fa" strokeWidth={2} dot={false} name="TX" />
      </LineChart>
    </ResponsiveContainer>
  )
}
