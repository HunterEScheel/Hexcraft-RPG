import {
  CASTING_TIMES,
  CONCENTRATION_EFFECTS,
  REPEATABLE_CRITERIA,
  SPELL_CASTING_TIMES,
  EP_PER_DAMAGE_DIE,
  SPELL_CRITERIA,
  SPELL_TARGETING_OPTIONS,
  applyConcentrationRule,
  concentrationModes,
  criterionApplies,
  criterionEp,
  criterionOptions,
  extrasFor,
  needsConcentration,
  selectionFor,
  selectedOption,
  withExtras,
  type CastingTimeKey,
  type CriterionKey,
  type SpellCriterion,
  type SpellDraft,
  type SpellSelection,
  type SpellTargeting,
} from '../system/spells'
import { conditionDescription } from '../system/conditions'
// Maneuvers alone may take no action at all.
import { isManeuverCriteria } from '../system/maneuvers'

/**
 * The parts every spell and maneuver is built from: the scaling criteria,
 * damage dice, timing and how it is resolved. Quick Cast, Quick Maneuver and
 * the Tools spell calculator all edit a SpellDraft through this.
 */
export function EffectBuilder({
  draft,
  onChange: onDraftChange,
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
  // Every change re-applies the concentration rule, so a spell that gains a
  // concentration effect or dice moves its duration onto that list.
  const onChange = (update: (d: SpellDraft) => SpellDraft) =>
    onDraftChange((d) => applyConcentrationRule(update(d), criteria))
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
      {criteria.filter((c) => criterionApplies(c, draft, criteria)).map((c) =>
        c.key === 'duration' && concentrationModes(c) ? (
          <DurationRow
            key={c.key}
            criterion={c}
            selection={selectionFor(draft, c.key)}
            concentration={needsConcentration(draft)}
            onPick={(modeIndex, optionIndex) =>
              onChange((d) => ({
                ...d,
                selections: { ...d.selections, duration: { modeIndex, optionIndex } },
              }))
            }
          />
        ) : REPEATABLE_CRITERIA.includes(c.key) ? (
          <RepeatableRows
            key={c.key}
            criterion={c}
            draft={draft}
            onModeChange={(idx) => setMode(c.key, idx)}
            onOptionChange={(idx) => setOption(c.key, idx)}
            onExtrasChange={(extras) => onChange((d) => withExtras(d, c.key, extras))}
          />
        ) : (
          <CriterionRow
            key={c.key}
            criterion={c}
            selection={selectionFor(draft, c.key)}
            onModeChange={(idx) => setMode(c.key, idx)}
            onOptionChange={(idx) => setOption(c.key, idx)}
          />
        ),
      )}

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
            {(isManeuverCriteria(criteria) ? CASTING_TIMES : SPELL_CASTING_TIMES).map((t) => (
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
          {SPELL_TARGETING_OPTIONS.filter((o) => !o.spellOnly || !isManeuverCriteria(criteria)).map((opt) => {
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

/**
 * A criterion that can be picked more than once (Buff/Debuff, a maneuver's
 * Self-debuff): the first pick, then any extras, each priced on its own.
 */
function RepeatableRows({
  criterion,
  draft,
  onModeChange,
  onOptionChange,
  onExtrasChange,
}: {
  criterion: SpellCriterion
  draft: SpellDraft
  onModeChange: (idx: number) => void
  onOptionChange: (idx: number) => void
  onExtrasChange: (next: SpellSelection[]) => void
}) {
  const extras = extrasFor(draft, criterion.key)
  const first = selectionFor(draft, criterion.key)
  const hasFirst = (selectedOption(criterion, first)?.tier ?? 0) > 0
  // A new extra starts on the first real effect, past "none".
  const fresh: SpellSelection =
    (criterion.modes?.length ?? 0) > 1 ? { modeIndex: 1, optionIndex: 0 } : { modeIndex: 0, optionIndex: 1 }
  const setExtra = (i: number, patch: Partial<SpellSelection>) =>
    onExtrasChange(extras.map((e, j) => (j === i ? { ...e, ...patch } : e)))

  return (
    <div className="space-y-3">
      <CriterionRow
        criterion={criterion}
        selection={first}
        onModeChange={onModeChange}
        onOptionChange={onOptionChange}
      />
      {extras.map((sel, i) => (
        <CriterionRow
          key={i}
          criterion={criterion}
          label={`${criterion.label} ${i + 2}`}
          selection={sel}
          onModeChange={(modeIndex) => setExtra(i, { modeIndex, optionIndex: 0 })}
          onOptionChange={(optionIndex) => setExtra(i, { optionIndex })}
          onRemove={() => onExtrasChange(extras.filter((_, j) => j !== i))}
        />
      ))}
      <button
        type="button"
        onClick={() => onExtrasChange([...extras, fresh])}
        disabled={!hasFirst}
        title={hasFirst ? undefined : `Pick the first ${criterion.label.toLowerCase()} before adding another`}
        className="rounded border border-dashed border-zinc-700 px-2 py-1 text-xs text-zinc-400 hover:border-zinc-500 hover:text-zinc-200 disabled:opacity-40 disabled:cursor-not-allowed"
      >
        + Add another {criterion.label.toLowerCase()}
      </button>
    </div>
  )
}

interface CriterionRowProps {
  criterion: SpellCriterion
  selection: SpellSelection
  onModeChange: (idx: number) => void
  onOptionChange: (idx: number) => void
  /** Overrides the criterion's own label (e.g. "Buff / Debuff 2"). */
  label?: string
  /** Shows a remove button, for a repeated pick. */
  onRemove?: () => void
}

export function CriterionRow({
  criterion,
  selection,
  onModeChange,
  onOptionChange,
  label,
  onRemove,
}: CriterionRowProps) {
  const ep = criterionEp(criterion, selection)
  const hasModes = (criterion.modes?.length ?? 0) > 1
  const options = criterionOptions(criterion, selection.modeIndex)
  const currentOption = selectedOption(criterion, selection)

  return (
    <div>
      <div className="flex items-baseline justify-between mb-1">
        <span className="text-xs uppercase tracking-wide text-zinc-400">
          {label ?? criterion.label}
        </span>
        <span className="flex items-baseline gap-2 text-xs text-zinc-500 font-mono">
          {currentOption ? currentOption.label : '—'} ·{' '}
          {criterion.multiplies ? `×${currentOption?.multiplier ?? 1}` : formatEp(ep)}
          {onRemove && (
            <button
              type="button"
              onClick={onRemove}
              aria-label={`Remove ${label ?? criterion.label}`}
              className="text-zinc-500 hover:text-rose-400"
            >
              ✕
            </button>
          )}
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
              title={conditionDescription(opt.label)}
              className={
                'rounded px-2 py-1 text-xs border ' +
                (active
                  ? 'border-emerald-500 bg-emerald-900/30 text-emerald-200'
                  : 'border-zinc-700 bg-zinc-900 text-zinc-300 hover:border-zinc-500')
              }
            >
              <span>{opt.label}</span>
              {criterion.key === 'buffDebuff' && CONCENTRATION_EFFECTS.has(opt.label) && (
                <span
                  title="Needs concentration unless instantaneous"
                  className="ml-1 rounded bg-violet-900/50 px-1 text-[9px] font-semibold text-violet-200"
                >
                  C
                </span>
              )}
              <span className="ml-1 text-[10px] text-zinc-500 font-mono">
                {criterion.multiplies ? `×${opt.multiplier ?? 1}` : formatEp(optionEp)}
              </span>
            </button>
          )
        })}
      </div>
      {currentOption && conditionDescription(currentOption.label) && (
        <p className="mt-1 text-xs text-zinc-400">
          <span className="text-zinc-300">{currentOption.label}:</span>{' '}
          {conditionDescription(currentOption.label)}
        </p>
      )}
    </div>
  )
}

/**
 * A spell's duration under the concentration rule: with a concentration
 * effect (marked C) or dice of damage or healing it is instantaneous or held
 * by concentration; otherwise it lasts on its own. Only the allowed list is
 * offered.
 */
function DurationRow({
  criterion,
  selection,
  concentration,
  onPick,
}: {
  criterion: SpellCriterion
  selection: SpellSelection
  concentration: boolean
  onPick: (modeIndex: number, optionIndex: number) => void
}) {
  const modes = concentrationModes(criterion)!
  const standard = criterionOptions(criterion, modes.standard)
  const choices = concentration
    ? [
        { modeIndex: modes.standard, optionIndex: 0, option: standard[0] },
        ...criterionOptions(criterion, modes.concentration).map((option, i) => ({
          modeIndex: modes.concentration,
          optionIndex: i,
          option,
        })),
      ]
    : standard.map((option, i) => ({ modeIndex: modes.standard, optionIndex: i, option }))
  const current = selectedOption(criterion, selection)
  const ep = criterionEp(criterion, selection)

  return (
    <div>
      <div className="flex items-baseline justify-between mb-1">
        <span className="text-xs uppercase tracking-wide text-zinc-400">
          {criterion.label}
          <span
            className={
              'ml-2 rounded px-1.5 py-0.5 text-[10px] normal-case tracking-normal ' +
              (concentration
                ? 'bg-violet-900/40 text-violet-200'
                : 'bg-zinc-800 text-zinc-400')
            }
          >
            {concentration ? 'Concentration' : 'No concentration'}
          </span>
        </span>
        <span className="text-xs text-zinc-500 font-mono">
          {current ? current.label : '—'} · {formatEp(ep)}
        </span>
      </div>
      <div className="flex flex-wrap gap-1">
        {choices.map((c) => {
          const active =
            c.modeIndex === selection.modeIndex && c.optionIndex === selection.optionIndex
          return (
            <button
              key={`${c.modeIndex}-${c.optionIndex}`}
              type="button"
              onClick={() => onPick(c.modeIndex, c.optionIndex)}
              className={
                'rounded px-2 py-1 text-xs border ' +
                (active
                  ? 'border-emerald-500 bg-emerald-900/30 text-emerald-200'
                  : 'border-zinc-700 bg-zinc-900 text-zinc-300 hover:border-zinc-500')
              }
            >
              <span>{c.option.label}</span>
              <span className="ml-1 text-[10px] text-zinc-500 font-mono">
                {formatEp(c.option.tier * criterion.epPerTier)}
              </span>
            </button>
          )
        })}
      </div>
      <p className="mt-1 text-[11px] text-zinc-500">
        {concentration
          ? 'Has an area (splash or line), a concentration effect (marked C), or damage or healing dice, so anything longer than instantaneous needs concentration.'
          : 'No area, no concentration effect (marked C) and no dice, so it lasts without concentration.'}
      </p>
    </div>
  )
}

// Refunds read as "−5 EP".
function formatEp(ep: number): string {
  return ep < 0 ? `−${-ep} EP` : `${ep} EP`
}
