import { useEffect, useState } from 'react'
import { REST_ATTRIBUTES, type RestAttribute } from '../system/attributes'
import { rollLongRest } from '../system/character'

const ABBR: Record<RestAttribute, string> = { Intelligence: 'INT', Sense: 'SEN', Influence: 'INF' }

/**
 * A long rest heals 1d6 HP and recovers 1d4−1 EP per hour slept, plus the
 * character's rest attribute (Intelligence, Sense or Influence) once. Players
 * roll their own dice and type the totals, or let the app roll.
 */
export function LongRestDialog({
  restAttribute,
  attributes,
  onRestAttributeChange,
  onRest,
  onClose,
}: {
  restAttribute: RestAttribute
  attributes: Record<string, number>
  onRestAttributeChange: (a: RestAttribute) => void
  onRest: (hp: number, ep: number) => void
  onClose: () => void
}) {
  const bonus = attributes[restAttribute] ?? 0
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
  // Even with no dice, the rest attribute still counts.
  const ready = hp !== '' || ep !== '' || bonus > 0

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
          <span className="font-mono text-sky-300">1d4−1</span> ×{hours} for EP, then add your{' '}
          rest attribute once.
        </p>

        <div className="space-y-1">
          <span className="text-xs text-zinc-400">Rest attribute</span>
          <div className="flex gap-1">
            {REST_ATTRIBUTES.map((a) => (
              <button
                key={a}
                type="button"
                onClick={() => onRestAttributeChange(a)}
                className={
                  'flex-1 rounded border px-2 py-1 text-xs ' +
                  (a === restAttribute
                    ? 'border-sky-500 bg-sky-900/30 text-sky-200'
                    : 'border-zinc-700 text-zinc-400 hover:border-zinc-500')
                }
              >
                {ABBR[a]} <span className="font-mono text-[10px] text-zinc-500">{fmt(attributes[a] ?? 0)}</span>
              </button>
            ))}
          </div>
        </div>

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
            EP dice
            <input
              type="number"
              min={0}
              value={ep}
              onChange={(e) => setEp(e.target.value)}
              className="mt-1 w-full rounded border border-zinc-700 bg-zinc-950 px-2 py-1 text-center font-mono text-sky-300"
            />
          </label>
        </div>

        <p className="text-xs text-zinc-500 font-mono">
          EP: {num(ep)} dice {bonus >= 0 ? '+' : '−'} {Math.abs(bonus)} {ABBR[restAttribute]} ={' '}
          <span className="text-sky-300">{Math.max(0, num(ep) + bonus)}</span>
        </p>

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
              onRest(num(hp), Math.max(0, num(ep) + bonus))
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

function fmt(n: number): string {
  return n >= 0 ? `+${n}` : `${n}`
}
