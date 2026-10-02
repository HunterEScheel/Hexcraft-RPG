import type { ReactNode } from 'react'
import {
  CASTING_TIMES,
  SPELL_CRITERIA,
  criterionApplies,
  savedSpellCost,
  selectedOption,
  selectionFor,
  targetingName,
  type SpellCriterion,
  type SpellDraft,
} from '../system/spells'

export interface SavedEffectGroup {
  key: string
  label: string
  /** Save slots: the group's school or skill level. */
  limit: number
  items: {
    id: string
    name: string
    /** What it's drawn from, e.g. school · medium. */
    source: ReactNode
    bonus: number
    draft: SpellDraft
  }[]
}

/** Saved spells or maneuvers, grouped by school or skill, each usable at 25% off. */
export function SavedEffects({
  groups,
  verb,
  criteria = SPELL_CRITERIA,
  extraDice = false,
  currentEp,
  onUse,
  onRemove,
}: {
  groups: SavedEffectGroup[]
  verb: string
  criteria?: readonly SpellCriterion[]
  /** Maneuver dice add to the weapon's damage, shown as "+2d6". */
  extraDice?: boolean
  currentEp: number
  onUse: (epCost: number) => void
  onRemove: (id: string) => void
}) {
  return (
    <div className="space-y-3">
      {groups
        .filter((g) => g.items.length > 0)
        .map((group) => (
          <div key={group.key}>
            <div className="flex items-baseline justify-between mb-1">
              <h4 className="text-xs uppercase tracking-wide text-zinc-400">
                {group.label}
              </h4>
              <span className="text-xs text-zinc-500 font-mono">
                {group.items.length}/{group.limit} saved
              </span>
            </div>
            <ul className="space-y-1">
              {group.items.map((item) => (
                <SavedEffectRow
                  key={item.id}
                  item={item}
                  verb={verb}
                  criteria={criteria}
                  extraDice={extraDice}
                  currentEp={currentEp}
                  onUse={onUse}
                  onRemove={onRemove}
                />
              ))}
            </ul>
          </div>
        ))}
    </div>
  )
}

/** One saved spell or maneuver: its bonus, factors and a use button. */
export function SavedEffectRow({
  item,
  verb,
  criteria = SPELL_CRITERIA,
  extraDice = false,
  currentEp,
  onUse,
  onRemove,
}: {
  item: SavedEffectGroup['items'][number]
  verb: string
  criteria?: readonly SpellCriterion[]
  extraDice?: boolean
  currentEp: number
  onUse: (epCost: number) => void
  onRemove: (id: string) => void
}) {
  const cost = savedSpellCost(item.draft, criteria).totalEp
  const canUse = cost <= currentEp
  const factors: string[] = []
  for (const c of criteria) {
    if (!criterionApplies(c, item.draft, criteria)) continue
    const opt = selectedOption(c, selectionFor(item.draft, c.key))
    if (!opt || (c.refund && c.key === 'selfDebuff' && opt.tier === 0)) continue
    factors.push(c.key === 'selfDebuff' ? `self: ${opt.label}` : opt.label)
  }
  if (item.draft.damageDice > 0)
    factors.push(`${extraDice ? '+' : ''}${item.draft.damageDice}d6`)
  const time = CASTING_TIMES.find((t) => t.key === item.draft.castingTime)
  if (time) factors.push(`${time.label} (×${time.multiplier})`)
  const targeting = item.draft.targeting ?? 'hit'
  return (
    <li
      className="rounded bg-zinc-900 border border-zinc-800 px-3 py-2 flex items-center gap-2"
    >
      <span className="text-sm text-zinc-100 whitespace-nowrap">
        {item.name}
      </span>
      <span className="text-[11px] text-zinc-400 whitespace-nowrap">
        {item.source}
      </span>
      <span className="text-xs font-mono text-amber-300 whitespace-nowrap">
        {targeting === 'hit'
          ? `hit ${item.bonus >= 0 ? '+' : ''}${item.bonus}`
          : `${targetingName(targeting)} DC ${10 + item.bonus}`}
      </span>
      <div className="flex flex-wrap gap-1 flex-1 min-w-0">
        {factors.map((label, i) => (
          <span
            key={i}
            className="rounded border border-zinc-700 bg-zinc-950 px-2 py-0.5 text-[11px] text-zinc-300"
          >
            {label}
          </span>
        ))}
      </div>
      <button
        type="button"
        onClick={() => onUse(cost)}
        disabled={!canUse}
        title={
          !canUse
            ? `Not enough EP (need ${cost}, have ${currentEp})`
            : undefined
        }
        className="rounded bg-violet-500 hover:bg-violet-400 disabled:opacity-40 disabled:cursor-not-allowed px-3 py-1 text-xs font-medium text-zinc-950 whitespace-nowrap"
      >
        {verb} (−{cost})
      </button>
      <button
        type="button"
        onClick={() => onRemove(item.id)}
        className="text-zinc-500 hover:text-rose-400 text-sm"
        aria-label={`Remove ${item.name}`}
      >
        ✕
      </button>
    </li>
  )
}
