import { useMemo, useState } from 'react'
import { emptySpellDraft, isEmptyDraft, spellCost } from '../system/spells'
import { MANEUVER_CRITERIA, MANEUVER_SKILLS } from '../system/maneuvers'
import {
  monsterManeuverBonus,
  spellTargetingLabel,
  type Monster,
  type MonsterManeuver,
} from '../system/monster'
import { EffectBuilder, PickerSelect } from './EffectBuilder'

/**
 * Maneuvers for a monster block: built like a character's, from one of the
 * monster's attack skills. Monsters need no training or save slots for them;
 * using one still costs EP.
 */
export function MonsterManeuverComposer({
  monster,
  value,
  onChange,
}: {
  monster: Monster
  value: MonsterManeuver[]
  onChange: (next: MonsterManeuver[]) => void
}) {
  const [draft, setDraft] = useState(emptySpellDraft)
  const [name, setName] = useState('')
  const [skillId, setSkillId] = useState('')
  const cost = useMemo(() => spellCost(draft, MANEUVER_CRITERIA), [draft])
  const skillName = (id: string) => MANEUVER_SKILLS.find((s) => s.id === id)?.name ?? id

  const canAdd = name.trim().length > 0 && skillId !== '' && !isEmptyDraft(draft)

  const handleAdd = () => {
    if (!canAdd) return
    onChange([
      ...value,
      { id: `monster-maneuver-${crypto.randomUUID()}`, name: name.trim(), skillId, draft },
    ])
    setName('')
    setDraft(emptySpellDraft())
  }

  return (
    <div className="space-y-4">
      <PickerSelect
        label="Combat skill"
        value={skillId}
        options={MANEUVER_SKILLS.map((s) => ({
          value: s.id,
          label: `${s.name} (bonus ${monsterManeuverBonus(monster, s.id)})`,
        }))}
        onChange={setSkillId}
      />
      <EffectBuilder
        draft={draft}
        onChange={setDraft}
        timingLabel="Timing"
        criteria={MANEUVER_CRITERIA}
        damageLabel="Extra damage (d6, on top of the weapon's)"
      />

      <div className="rounded border border-zinc-800 bg-zinc-950 p-3 flex items-center gap-2 flex-wrap">
        <span className="text-xs text-zinc-500 font-mono whitespace-nowrap">
          {cost.baseEp} base × {cost.multiplier} = {cost.totalEp} EP
        </span>
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Maneuver name…"
          className="flex-1 min-w-[10rem] bg-zinc-900 border border-zinc-700 rounded px-2 py-1.5 text-sm text-zinc-100"
        />
        <button
          type="button"
          onClick={handleAdd}
          disabled={!canAdd}
          title={
            skillId === ''
              ? 'Pick a combat skill'
              : name.trim().length === 0
                ? 'Give the maneuver a name'
                : isEmptyDraft(draft)
                  ? 'Set at least one criterion or damage die'
                  : undefined
          }
          className="rounded bg-emerald-500 hover:bg-emerald-400 disabled:opacity-40 disabled:cursor-not-allowed px-4 py-1.5 text-sm font-medium text-zinc-950"
        >
          Add maneuver
        </button>
      </div>

      {value.length > 0 && (
        <ul className="space-y-1">
          {value.map((m) => (
            <li
              key={m.id}
              className="rounded bg-zinc-900 border border-zinc-800 px-3 py-2 flex items-center gap-2"
            >
              <span className="text-sm text-zinc-100 whitespace-nowrap">{m.name}</span>
              <span className="text-[11px] text-rose-300 whitespace-nowrap">{skillName(m.skillId)}</span>
              <span className="text-xs font-mono text-amber-300 whitespace-nowrap">
                {spellTargetingLabel(m, monsterManeuverBonus(monster, m.skillId))}
              </span>
              <span className="text-xs font-mono text-zinc-500 flex-1">
                {spellCost(m.draft, MANEUVER_CRITERIA).totalEp} EP
              </span>
              <button
                type="button"
                onClick={() => onChange(value.filter((x) => x.id !== m.id))}
                className="text-zinc-500 hover:text-rose-400 text-sm"
                aria-label={`Remove ${m.name}`}
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
