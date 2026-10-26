import { useState } from 'react'
import { maneuverBonus, maneuverReach, combatSkillLevel, type Character } from '../system/character'
import { MANEUVER_CRITERIA, MANEUVER_SKILLS, maneuverCriteria } from '../system/maneuvers'
import type { SpellDraft } from '../system/spells'
import { PickerSelect } from './EffectBuilder'
import { QuickEffect } from './QuickEffect'

/**
 * Build and use a maneuver from any attack skill, trained or not: the skill
 * sets its bonus. Saving one still takes a slot per level in the skill.
 */
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
  const skill = MANEUVER_SKILLS.find((s) => s.id === skillId)
  const saved = character.savedManeuvers ?? []
  return (
    <QuickEffect
      noun="maneuver"
      verb="Use"
      timingLabel="Timing"
      criteria={(draft) =>
        skill ? maneuverCriteria(skill.id, maneuverReach(character, skill.id), draft.castingTime) : MANEUVER_CRITERIA
      }
      damageLabel="Extra damage (d6, on top of the weapon's)"
      pickers={
        <PickerSelect
          label="Combat skill"
          value={skillId}
          options={MANEUVER_SKILLS.map((s) => ({
            value: s.id,
            label: `${s.name} (Lv ${combatSkillLevel(character, s.id)}${s.attribute ? ` + ${s.attribute}` : ''})`,
          }))}
          onChange={setSkillId}
        />
      }
      pickerHint={skill ? null : 'Pick a combat skill'}
      pickFirst
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
