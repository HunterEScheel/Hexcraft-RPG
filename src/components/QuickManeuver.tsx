import { useState } from 'react'
import { maneuverBonus, combatSkillLevel, type Character } from '../system/character'
import { MANEUVER_SKILLS } from '../system/maneuvers'
import type { SpellDraft } from '../system/spells'
import { PickerSelect } from './EffectBuilder'
import { QuickEffect } from './QuickEffect'

/** Build and use a maneuver: a trained attack skill sets its bonus. */
export function QuickManeuver({
  character,
  onUse,
  onSave,
}: {
  character: Character
  onUse: (epCost: number) => void
  onSave: (maneuver: { name: string; skillId: string; draft: SpellDraft }) => void
}) {
  const [skillId, setSkillId] = useState('')
  const trained = MANEUVER_SKILLS.filter((s) => combatSkillLevel(character, s.id) > 0)

  if (trained.length === 0) {
    return (
      <p className="text-sm text-zinc-500 italic">
        Train an attack skill (1-handed melee, Grapple, …) to use maneuvers.
      </p>
    )
  }

  const skill = trained.find((s) => s.id === skillId)
  const saved = character.savedManeuvers ?? []
  return (
    <QuickEffect
      noun="maneuver"
      verb="Use"
      timingLabel="Timing"
      pickers={
        <PickerSelect
          label="Combat skill"
          value={skillId}
          options={trained.map((s) => ({
            value: s.id,
            label: `${s.name} (Lv ${combatSkillLevel(character, s.id)}${s.attribute ? ` + ${s.attribute}` : ''})`,
          }))}
          onChange={setSkillId}
        />
      }
      pickerHint={skill ? null : 'Pick a combat skill'}
      bonus={skill ? maneuverBonus(character, skill.id) : null}
      currentEp={character.currentEp}
      slots={
        skill
          ? {
              label: skill.name,
              used: saved.filter((m) => m.skillId === skill.id).length,
              limit: combatSkillLevel(character, skill.id),
            }
          : null
      }
      onUse={onUse}
      onSave={(name, draft) => onSave({ name, skillId, draft })}
    />
  )
}
