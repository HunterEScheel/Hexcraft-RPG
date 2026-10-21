import { useMemo, useState } from 'react'
import { emptySpellDraft, spellCost } from '../system/spells'
import {
  MAGIC_MEDIUMS,
  MAGIC_SCHOOLS,
  type MagicMedium,
  type MagicSchool,
} from '../system/magicSchools'
import { spellDamage, spellFactors, spellTargetingLabel, type MonsterSpell } from '../system/monster'
import { EffectBuilder, PickerSelect } from './EffectBuilder'

interface Props {
  schools: Record<MagicSchool, number>
  mediums: Record<MagicMedium, number>
  value: MonsterSpell[]
  onChange: (next: MonsterSpell[]) => void
}

/**
 * A monster's spells, built from a school and medium it knows (their levels
 * set the bonus). Unlike a character, a monster can keep any number.
 */
export function MonsterSpellComposer({ schools, mediums, value, onChange }: Props) {
  const [draft, setDraft] = useState(emptySpellDraft)
  const [name, setName] = useState('')
  const [school, setSchool] = useState<MagicSchool | ''>('')
  const [medium, setMedium] = useState<MagicMedium | ''>('')

  const known = {
    schools: MAGIC_SCHOOLS.filter((s) => (schools[s] ?? 0) > 0),
    mediums: MAGIC_MEDIUMS.filter((m) => (mediums[m] ?? 0) > 0),
  }
  // A pick the monster has since forgotten no longer counts.
  const pickedSchool = school !== '' && known.schools.includes(school) ? school : ''
  const pickedMedium = medium !== '' && known.mediums.includes(medium) ? medium : ''
  const bonus = (s: { school: MagicSchool; medium: MagicMedium }) =>
    (schools[s.school] ?? 0) + (mediums[s.medium] ?? 0)

  const cost = useMemo(() => spellCost(draft), [draft])
  const hint =
    pickedSchool === ''
      ? 'Pick a school'
      : pickedMedium === ''
        ? 'Pick a medium'
        : name.trim().length === 0
          ? 'Give the spell a name'
          : cost.totalEp === 0
            ? 'Set at least one criterion or damage die'
            : null

  if (known.schools.length === 0 || known.mediums.length === 0) {
    return (
      <p className="text-sm text-zinc-500 italic">
        Give the monster at least one school and one medium above to build spells.
      </p>
    )
  }

  const handleAdd = () => {
    if (hint !== null || pickedSchool === '' || pickedMedium === '') return
    onChange([
      ...value,
      {
        id: `monster-spell-${crypto.randomUUID()}`,
        name: name.trim(),
        school: pickedSchool,
        medium: pickedMedium,
        draft,
      },
    ])
    setName('')
    setDraft(emptySpellDraft())
  }

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-3">
        <PickerSelect
          label="School"
          value={pickedSchool}
          options={known.schools.map((s) => ({ value: s, label: `${s} (Lv ${schools[s]})` }))}
          onChange={(v) => setSchool(v as MagicSchool)}
        />
        <PickerSelect
          label="Medium"
          value={pickedMedium}
          options={known.mediums.map((m) => ({ value: m, label: `${m} (Lv ${mediums[m]})` }))}
          onChange={(v) => setMedium(v as MagicMedium)}
        />
      </div>

      <EffectBuilder draft={draft} onChange={setDraft} />

      <div className="rounded border border-zinc-800 bg-zinc-950 p-3 flex items-center gap-2 flex-wrap">
        <span className="text-xs text-zinc-500 font-mono whitespace-nowrap">
          {cost.baseEp} base × {cost.multiplier} = {cost.totalEp} EP
          {pickedSchool !== '' && pickedMedium !== '' && (
            <>
              {' · '}
              <span className="text-amber-300">
                {spellTargetingLabel({ draft }, bonus({ school: pickedSchool, medium: pickedMedium }))}
              </span>
            </>
          )}
        </span>
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Spell name…"
          className="flex-1 min-w-[10rem] bg-zinc-900 border border-zinc-700 rounded px-2 py-1.5 text-sm text-zinc-100"
        />
        <button
          type="button"
          onClick={handleAdd}
          disabled={hint !== null}
          title={hint ?? undefined}
          className="rounded bg-emerald-500 hover:bg-emerald-400 disabled:opacity-40 disabled:cursor-not-allowed px-4 py-1.5 text-sm font-medium text-zinc-950"
        >
          Add spell
        </button>
      </div>

      {value.length > 0 && (
        <ul className="space-y-1">
          {value.map((s) => (
            <li
              key={s.id}
              className="rounded bg-zinc-900 border border-zinc-800 px-3 py-2 flex items-center gap-2"
            >
              <span className="text-sm text-zinc-100 whitespace-nowrap">{s.name}</span>
              <span className="text-[11px] text-violet-300 whitespace-nowrap">
                {s.school} · {s.medium}
              </span>
              <span className="text-xs font-mono text-amber-300 whitespace-nowrap">
                {spellTargetingLabel(s, bonus(s))}
              </span>
              {spellDamage(s) && (
                <span className="text-xs font-mono text-rose-300 whitespace-nowrap">
                  {spellDamage(s)} damage
                </span>
              )}
              <span className="text-xs font-mono text-zinc-500 whitespace-nowrap">
                {spellCost(s.draft).totalEp} EP
              </span>
              <span className="flex flex-1 flex-wrap gap-1 min-w-0">
                {spellFactors(s).map((label, i) => (
                  <span
                    key={i}
                    className="rounded border border-zinc-700 bg-zinc-950 px-1.5 py-0.5 text-[10px] text-zinc-400"
                  >
                    {label}
                  </span>
                ))}
              </span>
              <button
                type="button"
                onClick={() => onChange(value.filter((x) => x.id !== s.id))}
                className="text-zinc-500 hover:text-rose-400 text-sm"
                aria-label={`Remove ${s.name}`}
              >
                ✕
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
