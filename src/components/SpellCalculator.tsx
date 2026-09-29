import { useMemo, useState } from 'react'
import {
  CASTING_TIMES,
  EP_PER_DAMAGE_DIE,
  SPELL_CRITERIA,
  SPELL_TARGETING_OPTIONS,
  emptySpellDraft,
  savedSpellCost,
  spellCost,
  type CastingTimeKey,
  type CriterionKey,
} from '../system/spells'
import { CriterionRow } from './QuickCast'
import { NumberStepper } from './NumberStepper'

/**
 * On-the-fly casting for the GM: the same spell builder as a character's
 * Quick Cast, but for any caster. Enter their school and medium levels and it
 * gives the EP cost, the saved-spell cost and the hit bonus or save DC. It
 * spends nothing and saves nothing.
 */
export function SpellCalculator() {
  const [draft, setDraft] = useState(emptySpellDraft)
  const [school, setSchool] = useState(0)
  const [medium, setMedium] = useState(0)

  const cost = useMemo(() => spellCost(draft), [draft])
  const saved = useMemo(() => savedSpellCost(draft), [draft])
  const bonus = school + medium
  const targeting = SPELL_TARGETING_OPTIONS.find((t) => t.key === draft.targeting)

  const setMode = (key: CriterionKey, modeIndex: number) =>
    setDraft((d) => ({
      ...d,
      selections: { ...d.selections, [key]: { modeIndex, optionIndex: 0 } },
    }))
  const setOption = (key: CriterionKey, optionIndex: number) =>
    setDraft((d) => ({
      ...d,
      selections: {
        ...d.selections,
        [key]: { ...d.selections[key], optionIndex },
      },
    }))

  return (
    <section className="overflow-hidden rounded-lg border border-violet-700/40 bg-zinc-900/50">
      <header className="flex items-baseline justify-between border-b border-zinc-800 bg-zinc-950/50 px-4 py-2">
        <h3 className="text-[10px] font-medium uppercase tracking-[0.3em] text-violet-300/80">
          Spell Calculator
        </h3>
        <button
          type="button"
          onClick={() => setDraft(emptySpellDraft())}
          className="text-[10px] uppercase tracking-[0.2em] text-zinc-500 hover:text-zinc-300"
        >
          Reset
        </button>
      </header>

      <div className="space-y-4 p-4">
        <div className="flex flex-wrap gap-x-8 gap-y-2">
          <NumberStepper label="School level" value={school} onChange={setSchool} min={0} />
          <NumberStepper label="Medium level" value={medium} onChange={setMedium} min={0} />
        </div>

        {SPELL_CRITERIA.map((c) => (
          <CriterionRow
            key={c.key}
            criterion={c}
            selection={draft.selections[c.key]}
            onModeChange={(idx) => setMode(c.key, idx)}
            onOptionChange={(idx) => setOption(c.key, idx)}
          />
        ))}

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <div className="mb-1 flex items-baseline justify-between">
              <span className="text-xs uppercase tracking-wide text-zinc-400">
                Damage dice
              </span>
              <span className="font-mono text-xs text-zinc-500">
                {EP_PER_DAMAGE_DIE} EP per die
              </span>
            </div>
            <NumberStepper
              value={draft.damageDice}
              onChange={(damageDice) => setDraft((d) => ({ ...d, damageDice }))}
              min={0}
            />
          </div>
          <div>
            <span className="mb-1 block text-xs uppercase tracking-wide text-zinc-400">
              Casting time
            </span>
            <select
              value={draft.castingTime}
              onChange={(e) =>
                setDraft((d) => ({ ...d, castingTime: e.target.value as CastingTimeKey }))
              }
              className="w-full rounded border border-zinc-700 bg-zinc-900 px-3 py-2 text-sm text-zinc-100"
            >
              {CASTING_TIMES.map((t) => (
                <option key={t.key} value={t.key}>
                  {t.label} (×{t.multiplier})
                </option>
              ))}
            </select>
          </div>
        </div>

        <div>
          <span className="mb-1 block text-xs uppercase tracking-wide text-zinc-400">
            Resolved by
          </span>
          <div className="flex flex-wrap gap-1">
            {SPELL_TARGETING_OPTIONS.map((opt) => (
              <button
                key={opt.key}
                type="button"
                onClick={() => setDraft((d) => ({ ...d, targeting: opt.key }))}
                className={
                  'rounded border px-3 py-1.5 text-xs ' +
                  (draft.targeting === opt.key
                    ? 'border-amber-500 bg-amber-900/30 text-amber-200'
                    : 'border-zinc-700 bg-zinc-900 text-zinc-300 hover:border-zinc-500')
                }
              >
                {opt.label}
              </button>
            ))}
          </div>
        </div>

        <div className="grid grid-cols-3 gap-3 rounded border border-zinc-800 bg-zinc-950 p-3 text-center">
          <Result label="EP" value={cost.totalEp} note={`${cost.baseEp} base × ${cost.multiplier}`} />
          <Result label="Saved" value={saved.totalEp} note="−25% if prepared" />
          {draft.targeting === 'hit' ? (
            <Result label="Hit bonus" value={`${bonus >= 0 ? '+' : ''}${bonus}`} note="vs Evasion" />
          ) : (
            <Result label={`${targeting?.label ?? 'Save'} DC`} value={10 + bonus} note="10 + school + medium" />
          )}
        </div>
        <p className="text-xs text-zinc-500">
          Amping: every extra 5 EP raises the hit bonus or DC by 1.
        </p>
      </div>
    </section>
  )
}

function Result({ label, value, note }: { label: string; value: number | string; note: string }) {
  return (
    <div>
      <div className="text-[10px] uppercase tracking-[0.2em] text-zinc-500">{label}</div>
      <div className="font-mono text-2xl text-amber-300">{value}</div>
      <div className="text-[10px] text-zinc-500">{note}</div>
    </div>
  )
}
