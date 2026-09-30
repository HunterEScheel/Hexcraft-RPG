import { useMemo, useState, type ReactNode } from 'react'
import {
  emptySpellDraft,
  isEmptyDraft,
  spellCost,
  targetingName,
  type SpellCriterion,
  type SpellDraft,
} from '../system/spells'
import { EffectBuilder } from './EffectBuilder'

/**
 * Build a spell or maneuver on the fly, then use it (spending its EP) or save
 * it. The wrapper supplies whatever picks the bonus (a school and medium, or a
 * combat skill) as `pickers`, and reports the result through `bonus`.
 */
export function QuickEffect({
  noun,
  verb,
  timingLabel,
  criteria,
  damageLabel,
  pickers,
  pickerHint,
  bonus,
  currentEp,
  slots,
  onUse,
  onSave,
}: {
  noun: string
  verb: string
  timingLabel?: string
  criteria?: readonly SpellCriterion[]
  damageLabel?: string
  pickers: ReactNode
  /** Why the effect can't be used yet (e.g. "Pick a school"), or null. */
  pickerHint: string | null
  /** Hit bonus from the picks; the save DC is 10 + this. Null until picked. */
  bonus: number | null
  currentEp: number
  /** Save slots for the current pick, or null when nothing is picked. */
  slots: { label: string; used: number; limit: number } | null
  onUse: (epCost: number) => void
  onSave: (name: string, draft: SpellDraft) => void
}) {
  const [draft, setDraft] = useState(emptySpellDraft)
  const [name, setName] = useState('')
  const cost = useMemo(() => spellCost(draft, criteria), [draft, criteria])
  const reset = () => setDraft(emptySpellDraft())

  // A maneuver whose drawbacks cover its whole price is free, but still has to
  // be something.
  const built = !isEmptyDraft(draft)
  const canUse = pickerHint === null && built && cost.totalEp <= currentEp
  const slotsLeft = slots ? Math.max(0, slots.limit - slots.used) : 0
  const canSave =
    pickerHint === null && name.trim().length > 0 && built && slotsLeft > 0

  return (
    <div className="space-y-4">
      {pickers}
      <EffectBuilder
        draft={draft}
        onChange={setDraft}
        timingLabel={timingLabel}
        criteria={criteria}
        damageLabel={damageLabel}
      />

      <div className="rounded border border-zinc-800 bg-zinc-950 p-3 space-y-2">
        <div className="flex items-center justify-between gap-3 flex-wrap">
          <div className="text-xs text-zinc-500 font-mono">
            {cost.baseEp} base × {cost.multiplier} = {cost.totalEp} EP
            {bonus !== null && (
              <>
                {' · '}
                <span className="text-amber-300">
                  {draft.targeting === 'hit'
                    ? `hit ${bonus >= 0 ? '+' : ''}${bonus}`
                    : `${targetingName(draft.targeting)} DC ${10 + bonus}`}
                </span>
              </>
            )}
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={reset}
              className="rounded bg-zinc-800 hover:bg-zinc-700 px-3 py-1.5 text-xs text-zinc-300"
            >
              Reset
            </button>
            <button
              type="button"
              onClick={() => {
                if (!canUse) return
                onUse(cost.totalEp)
                reset()
              }}
              disabled={!canUse}
              title={
                !built
                  ? 'Set at least one criterion or damage die'
                  : cost.totalEp > currentEp
                    ? `Not enough EP (need ${cost.totalEp}, have ${currentEp})`
                    : (pickerHint ?? undefined)
              }
              className="rounded bg-violet-500 hover:bg-violet-400 disabled:opacity-40 disabled:cursor-not-allowed px-4 py-1.5 text-sm font-medium text-zinc-950"
            >
              {verb} (−{cost.totalEp} EP)
            </button>
          </div>
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={`Name to save this ${noun}…`}
            className="flex-1 min-w-[12rem] bg-zinc-900 border border-zinc-700 rounded px-2 py-1.5 text-sm text-zinc-100"
          />
          {slots && slots.limit > 0 && (
            <span className="text-xs text-zinc-500 font-mono whitespace-nowrap">
              {slots.label}: {slots.used}/{slots.limit} saved
            </span>
          )}
          <button
            type="button"
            onClick={() => {
              if (!canSave) return
              onSave(name.trim(), draft)
              setName('')
              reset()
            }}
            disabled={!canSave}
            title={
              pickerHint ??
              (name.trim().length === 0
                ? `Give the ${noun} a name`
                : !built
                  ? 'Set at least one criterion or damage die'
                  : slotsLeft === 0 && slots
                    ? `No save slots left for ${slots.label} (level ${slots.limit})`
                    : undefined)
            }
            className="rounded bg-emerald-500 hover:bg-emerald-400 disabled:opacity-40 disabled:cursor-not-allowed px-4 py-1.5 text-sm font-medium text-zinc-950"
          >
            Save {noun}
          </button>
        </div>
      </div>
    </div>
  )
}
