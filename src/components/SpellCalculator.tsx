import { useMemo, useState } from 'react'
import {
  SPELL_TARGETING_OPTIONS,
  emptySpellDraft,
  savedSpellCost,
  spellCost,
} from '../system/spells'
import { MANEUVER_CRITERIA, MANEUVER_SKILLS } from '../system/maneuvers'
import {
  MAGIC_MEDIUMS,
  MAGIC_MEDIUM_DESCRIPTIONS,
  MAGIC_SCHOOLS,
} from '../system/magicSchools'
import { EffectBuilder } from './EffectBuilder'
import { NumberStepper } from './NumberStepper'

/**
 * On-the-fly casting for the GM: the same builder as a character's Quick Cast
 * and Quick Maneuver, but for anyone. Enter the two numbers behind the bonus
 * (school + medium for a spell, combat skill + attribute for a maneuver) and it
 * gives the EP cost, the saved cost and the hit bonus or save DC. It spends
 * nothing and saves nothing.
 */
export function SpellCalculator() {
  const [draft, setDraft] = useState(emptySpellDraft)
  const [maneuver, setManeuver] = useState(false)
  const [school, setSchool] = useState(0)
  const [medium, setMedium] = useState(0)
  // What the two numbers belong to: a school and medium, or an attack skill
  // (whose attribute is the second number). Labels only; the math is the same.
  const [schoolName, setSchoolName] = useState<string>(MAGIC_SCHOOLS[0])
  const [mediumName, setMediumName] = useState<string>(MAGIC_MEDIUMS[0])
  const [skillId, setSkillId] = useState<string>(MANEUVER_SKILLS[0].id)
  const skill = MANEUVER_SKILLS.find((s) => s.id === skillId) ?? MANEUVER_SKILLS[0]

  const criteria = maneuver ? MANEUVER_CRITERIA : undefined
  const cost = useMemo(() => spellCost(draft, criteria), [draft, criteria])
  const saved = useMemo(() => savedSpellCost(draft, criteria), [draft, criteria])
  const bonus = school + medium
  const targeting = SPELL_TARGETING_OPTIONS.find((t) => t.key === draft.targeting)

  return (
    <section className="overflow-hidden rounded-lg border border-violet-700/40 bg-zinc-900/50">
      <header className="flex items-baseline justify-between border-b border-zinc-800 bg-zinc-950/50 px-4 py-2">
        <div className="flex items-baseline gap-3">
          <h3 className="text-[10px] font-medium uppercase tracking-[0.3em] text-violet-300/80">
            {maneuver ? 'Maneuver' : 'Spell'} Calculator
          </h3>
          <div className="flex gap-1">
            {(['Spell', 'Maneuver'] as const).map((kind) => (
              <button
                key={kind}
                type="button"
                onClick={() => {
                  // The two use different option lists, so start the build over.
                  setManeuver(kind === 'Maneuver')
                  setDraft(emptySpellDraft())
                }}
                className={
                  'rounded px-2 py-0.5 text-[10px] uppercase tracking-wider border ' +
                  (maneuver === (kind === 'Maneuver')
                    ? 'border-violet-500 bg-violet-900/30 text-violet-200'
                    : 'border-zinc-700 text-zinc-500 hover:text-zinc-300')
                }
              >
                {kind}
              </button>
            ))}
          </div>
        </div>
        <button
          type="button"
          onClick={() => setDraft(emptySpellDraft())}
          className="text-[10px] uppercase tracking-[0.2em] text-zinc-500 hover:text-zinc-300"
        >
          Reset
        </button>
      </header>

      <div className="space-y-4 p-4">
        {maneuver ? (
          <Choices
            label="Combat skill"
            options={MANEUVER_SKILLS.map((s) => ({
              value: s.id,
              label: s.name,
              note: s.attribute,
            }))}
            value={skillId}
            onChange={setSkillId}
          />
        ) : (
          <>
            <Choices
              label="School"
              options={MAGIC_SCHOOLS.map((s) => ({ value: s, label: s }))}
              value={schoolName}
              onChange={setSchoolName}
            />
            <Choices
              label="Medium"
              options={MAGIC_MEDIUMS.map((m) => ({
                value: m,
                label: m,
                title: MAGIC_MEDIUM_DESCRIPTIONS[m],
              }))}
              value={mediumName}
              onChange={setMediumName}
            />
          </>
        )}
        <div className="flex flex-wrap gap-x-8 gap-y-2">
          <NumberStepper
            label={maneuver ? `${skill.name} level` : `${schoolName} level`}
            value={school}
            onChange={setSchool}
            min={0}
          />
          <NumberStepper
            label={maneuver ? (skill.attribute ?? 'Attribute') : `${mediumName} level`}
            value={medium}
            onChange={setMedium}
            min={maneuver ? -10 : 0}
          />
        </div>

        <EffectBuilder
          draft={draft}
          onChange={setDraft}
          timingLabel={maneuver ? 'Timing' : 'Casting time'}
          criteria={criteria}
          damageLabel={maneuver ? "Extra damage (d6, on top of the weapon's)" : undefined}
        />

        <div className="grid grid-cols-3 gap-3 rounded border border-zinc-800 bg-zinc-950 p-3 text-center">
          <Result label="EP" value={cost.totalEp} note={`${cost.baseEp} base × ${cost.multiplier}`} />
          <Result label="Saved" value={saved.totalEp} note="−25% if prepared" />
          {draft.targeting === 'hit' ? (
            <Result label="Hit bonus" value={`${bonus >= 0 ? '+' : ''}${bonus}`} note="vs Evasion" />
          ) : (
            <Result label={`${targeting?.label ?? 'Save'} DC`} value={10 + bonus} note={maneuver ? '10 + skill + attribute' : '10 + school + medium'} />
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

/** Every option on show at once, so nothing hides in a dropdown. */
function Choices({
  label,
  options,
  value,
  onChange,
}: {
  label: string
  options: { value: string; label: string; note?: string; title?: string }[]
  value: string
  onChange: (v: string) => void
}) {
  return (
    <div>
      <span className="mb-1 block text-xs uppercase tracking-wide text-zinc-400">{label}</span>
      <div className="flex flex-wrap gap-1">
        {options.map((o) => (
          <button
            key={o.value}
            type="button"
            title={o.title}
            onClick={() => onChange(o.value)}
            className={
              'rounded border px-2 py-1 text-xs ' +
              (o.value === value
                ? 'border-violet-500 bg-violet-900/30 text-violet-200'
                : 'border-zinc-700 bg-zinc-900 text-zinc-300 hover:border-zinc-500')
            }
          >
            {o.label}
            {o.note && <span className="ml-1 text-[10px] text-zinc-500">{o.note}</span>}
          </button>
        ))}
      </div>
    </div>
  )
}
