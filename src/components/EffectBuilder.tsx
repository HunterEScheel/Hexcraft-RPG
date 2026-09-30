import {
  CASTING_TIMES,
  EP_PER_DAMAGE_DIE,
  SPELL_CRITERIA,
  SPELL_TARGETING_OPTIONS,
  criterionApplies,
  criterionEp,
  criterionOptions,
  selectionFor,
  selectedOption,
  type CastingTimeKey,
  type CriterionKey,
  type SpellCriterion,
  type SpellDraft,
  type SpellSelection,
  type SpellTargeting,
} from '../system/spells'

/**
 * The parts every spell and maneuver is built from: the scaling criteria,
 * damage dice, timing and how it is resolved. Quick Cast, Quick Maneuver and
 * the Tools spell calculator all edit a SpellDraft through this.
 */
export function EffectBuilder({
  draft,
  onChange,
  timingLabel = 'Casting time',
  criteria = SPELL_CRITERIA,
  damageLabel = 'Damage dice (d6)',
}: {
  draft: SpellDraft
  onChange: (update: (d: SpellDraft) => SpellDraft) => void
  timingLabel?: string
  /** Maneuvers offer a shorter list than spells. */
  criteria?: readonly SpellCriterion[]
  damageLabel?: string
}) {
  const setMode = (key: CriterionKey, modeIndex: number) =>
    onChange((d) => ({
      ...d,
      selections: { ...d.selections, [key]: { modeIndex, optionIndex: 0 } },
    }))
  const setOption = (key: CriterionKey, optionIndex: number) =>
    onChange((d) => ({
      ...d,
      selections: {
        ...d.selections,
        [key]: { ...d.selections[key], optionIndex },
      },
    }))
  const setDamage = (n: number) =>
    onChange((d) => ({ ...d, damageDice: Math.max(0, n) }))
  const setTime = (k: CastingTimeKey) =>
    onChange((d) => ({ ...d, castingTime: k }))
  const setTargeting = (t: SpellTargeting) =>
    onChange((d) => ({ ...d, targeting: t }))

  return (
    <>
      {criteria.filter((c) => criterionApplies(c, draft, criteria)).map((c) => (
        <CriterionRow
          key={c.key}
          criterion={c}
          selection={selectionFor(draft, c.key)}
          onModeChange={(idx) => setMode(c.key, idx)}
          onOptionChange={(idx) => setOption(c.key, idx)}
        />
      ))}

      <div className="grid grid-cols-2 gap-3">
        <div>
          <div className="flex items-baseline justify-between mb-1">
            <span className="text-xs uppercase tracking-wide text-zinc-400">
              {damageLabel}
            </span>
            <span className="text-xs text-zinc-500 font-mono">
              {EP_PER_DAMAGE_DIE} EP per die
            </span>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setDamage(draft.damageDice - 1)}
              className="w-8 h-8 rounded bg-zinc-800 hover:bg-zinc-700 disabled:opacity-30"
              disabled={draft.damageDice <= 0}
            >
              −
            </button>
            <input
              type="number"
              min={0}
              max={50}
              value={draft.damageDice}
              onChange={(e) => setDamage(Number(e.target.value) || 0)}
              className="w-16 text-center bg-zinc-900 border border-zinc-700 rounded px-1 py-1 text-zinc-100"
            />
            <button
              type="button"
              onClick={() => setDamage(draft.damageDice + 1)}
              className="w-8 h-8 rounded bg-zinc-800 hover:bg-zinc-700"
            >
              +
            </button>
          </div>
        </div>

        <div>
          <span className="text-xs uppercase tracking-wide text-zinc-400 block mb-1">
            {timingLabel}
          </span>
          <select
            value={draft.castingTime}
            onChange={(e) => setTime(e.target.value as CastingTimeKey)}
            className="w-full bg-zinc-900 border border-zinc-700 rounded px-3 py-2 text-sm text-zinc-100"
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
        <span className="text-xs uppercase tracking-wide text-zinc-400 block mb-1">
          Targeting
        </span>
        <div className="flex flex-wrap gap-1">
          {SPELL_TARGETING_OPTIONS.map((opt) => {
            const active = draft.targeting === opt.key
            return (
              <button
                key={opt.key}
                type="button"
                onClick={() => setTargeting(opt.key)}
                className={
                  'rounded px-3 py-1.5 text-xs border ' +
                  (active
                    ? 'border-amber-500 bg-amber-900/30 text-amber-200'
                    : 'border-zinc-700 bg-zinc-900 text-zinc-300 hover:border-zinc-500')
                }
              >
                {opt.label}
              </button>
            )
          })}
        </div>
      </div>
    </>
  )
}

interface PickerSelectProps {
  label: string
  value: string
  options: { value: string; label: string }[]
  onChange: (v: string) => void
}

export function PickerSelect({
  label,
  value,
  options,
  onChange,
}: PickerSelectProps) {
  return (
    <div>
      <span className="text-xs uppercase tracking-wide text-zinc-400 block mb-1">
        {label}
      </span>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="w-full bg-zinc-900 border border-zinc-700 rounded px-3 py-2 text-sm text-zinc-100"
      >
        <option value="">— pick —</option>
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </div>
  )
}

interface CriterionRowProps {
  criterion: SpellCriterion
  selection: SpellSelection
  onModeChange: (idx: number) => void
  onOptionChange: (idx: number) => void
}

export function CriterionRow({
  criterion,
  selection,
  onModeChange,
  onOptionChange,
}: CriterionRowProps) {
  const ep = criterionEp(criterion, selection)
  const hasModes = (criterion.modes?.length ?? 0) > 1
  const options = criterionOptions(criterion, selection.modeIndex)
  const currentOption = selectedOption(criterion, selection)

  return (
    <div>
      <div className="flex items-baseline justify-between mb-1">
        <span className="text-xs uppercase tracking-wide text-zinc-400">
          {criterion.label}
        </span>
        <span className="text-xs text-zinc-500 font-mono">
          {currentOption ? currentOption.label : '—'} · {formatEp(ep)}
        </span>
      </div>
      {hasModes && criterion.modes && (
        <div className="flex flex-wrap gap-1 mb-2">
          {criterion.modes.map((mode, i) => {
            const active = i === selection.modeIndex
            return (
              <button
                key={mode.key}
                type="button"
                onClick={() => onModeChange(i)}
                className={
                  'rounded px-2 py-1 text-xs border ' +
                  (active
                    ? 'border-violet-500 bg-violet-900/30 text-violet-200'
                    : 'border-zinc-700 bg-zinc-950 text-zinc-400 hover:border-zinc-500')
                }
              >
                {mode.label}
              </button>
            )
          })}
        </div>
      )}
      <div className="flex flex-wrap gap-1">
        {options.map((opt, i) => {
          const active = i === selection.optionIndex
          const optionEp = (criterion.refund ? -1 : 1) * opt.tier * criterion.epPerTier
          return (
            <button
              key={i}
              type="button"
              onClick={() => onOptionChange(i)}
              className={
                'rounded px-2 py-1 text-xs border ' +
                (active
                  ? 'border-emerald-500 bg-emerald-900/30 text-emerald-200'
                  : 'border-zinc-700 bg-zinc-900 text-zinc-300 hover:border-zinc-500')
              }
            >
              <span>{opt.label}</span>
              <span className="ml-1 text-[10px] text-zinc-500 font-mono">
                {formatEp(optionEp)}
              </span>
            </button>
          )
        })}
      </div>
    </div>
  )
}

// Refunds read as "−5 EP".
function formatEp(ep: number): string {
  return ep < 0 ? `−${-ep} EP` : `${ep} EP`
}
