export default function StatusBadge({ online, label }) {
  return (
    <span className={`inline-flex items-center gap-1 text-xs font-medium px-2 py-0.5 rounded-full ${
      online ? 'bg-emerald-500/10 text-emerald-400' : 'bg-gray-700 text-gray-400'
    }`}>
      <span className={`w-1.5 h-1.5 rounded-full ${online ? 'bg-emerald-400 animate-pulse' : 'bg-gray-500'}`} />
      {label || (online ? '在线' : '离线')}
    </span>
  )
}
