import { useState } from 'react'

export default function DeployModal({ open, onClose, script }) {
  const [copied, setCopied] = useState(false)

  if (!open) return null

  const handleCopy = async () => {
    await navigator.clipboard.writeText(script)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-gray-900 border border-gray-700 rounded-lg w-full max-w-lg mx-4" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between p-4 border-b border-gray-700">
          <h2 className="text-sm font-bold">Agent 部署脚本</h2>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-200">&times;</button>
        </div>
        <div className="p-4">
          <pre className="bg-gray-950 text-gray-300 text-xs rounded p-3 overflow-x-auto whitespace-pre-wrap">{script}</pre>
        </div>
        <div className="flex justify-end p-4 border-t border-gray-700 gap-2">
          <button onClick={onClose} className="px-3 py-1.5 text-xs text-gray-400 hover:text-gray-200 transition-colors">关闭</button>
          <button onClick={handleCopy} className="px-3 py-1.5 text-xs bg-emerald-600 hover:bg-emerald-500 text-white rounded transition-colors">
            {copied ? '已复制' : '复制到剪贴板'}
          </button>
        </div>
      </div>
    </div>
  )
}
