import { useEffect, useState } from 'react'
import { rollLongRest } from '../system/character'

/**
 * A long rest heals 1d6 HP and recovers 1d4−1 EP per hour slept. Players roll
 * their own dice and type the totals, or let the app roll.
 */
export function LongRestDialog({
  onRest,
  onClose,
}: {
  onRest: (hp: number, ep: number) => void
  onClose: () => void
}) {
  const [hours, setHours] = useState(8)
  const [hp, setHp] = useState('')
  const [ep, setEp] = useState('')

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

  const roll = () => {
    const r = rollLongRest(hours)
    setHp(String(r.hp))
    setEp(String(r.ep))
  }
  const num = (v: string) => Math.max(0, Math.floor(Number(v) || 0))
  const ready = hp !== '' || ep !== ''

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="long-rest-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4"
      onClick={onClose}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="rounded-lg border border-emerald-500/40 bg-zinc-900 shadow-2xl max-w-xs w-full p-5 space-y-4"
      >
        <div className="flex items-center justify-between">
          <h3
            id="long-rest-title"
            className="text-[10px] font-medium uppercase tracking-[0.3em] text-zinc-500"
          >
            Long rest
          </h3>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            className="text-zinc-600 hover:text-zinc-200 text-sm leading-none"
          >
            ✕
          </button>
        </div>

        <label className="flex items-center justify-between gap-3 text-sm text-zinc-300">
          Hours slept
          <input
            type="number"
            min={0}
            value={hours}
            onChange={(e) => setHours(num(e.target.value))}
            className="w-20 rounded border border-zinc-700 bg-zinc-950 px-2 py-1 text-center text-zinc-100"
          />
        </label>
        <p className="text-xs text-zinc-500">
          Roll <span className="font-mono text-rose-300">{hours}d6</span> for HP and{' '}
          <span className="font-mono text-sky-300">1d4−1</span> ×{hours} for EP.
        </p>

        <div className="grid grid-cols-2 gap-3">
          <label className="text-xs text-zinc-400">
            HP healed
            <input
              type="number"
              min={0}
              value={hp}
              onChange={(e) => setHp(e.target.value)}
              className="mt-1 w-full rounded border border-zinc-700 bg-zinc-950 px-2 py-1 text-center font-mono text-rose-300"
            />
          </label>
          <label className="text-xs text-zinc-400">
            EP recovered
            <input
              type="number"
              min={0}
              value={ep}
              onChange={(e) => setEp(e.target.value)}
              className="mt-1 w-full rounded border border-zinc-700 bg-zinc-950 px-2 py-1 text-center font-mono text-sky-300"
            />
          </label>
        </div>

        <div className="flex items-center justify-between gap-2">
          <button
            type="button"
            onClick={roll}
            className="rounded bg-zinc-800 hover:bg-zinc-700 px-3 py-1.5 text-xs text-zinc-300"
          >
            Roll for me
          </button>
          <button
            type="button"
            disabled={!ready}
            onClick={() => {
              onRest(num(hp), num(ep))
              onClose()
            }}
            className="rounded bg-emerald-600 hover:bg-emerald-500 disabled:opacity-40 px-4 py-1.5 text-sm font-medium text-zinc-950"
          >
            Rest
          </button>
        </div>
      </div>
    </div>
  )
}
