import { combatSkillLevel, maneuverBonus, type Character } from '../system/character'
import { MANEUVER_SKILLS } from '../system/maneuvers'
import { SavedEffects } from './SavedEffects'

export function SavedManeuvers({
  character,
  onUse,
  onRemove,
}: {
  character: Character
  onUse: (epCost: number) => void
  onRemove: (id: string) => void
}) {
  const saved = character.savedManeuvers ?? []
  if (saved.length === 0) {
    return (
      <p className="text-sm text-zinc-500 italic">
        No saved maneuvers yet. Build one above and click Save maneuver to make
        it a signature move at a 25% cost reduction.
      </p>
    )
  }

  return (
    <SavedEffects
      verb="Use"
      currentEp={character.currentEp}
      onUse={onUse}
      onRemove={onRemove}
      groups={MANEUVER_SKILLS.map((skill) => ({
        key: skill.id,
        label: skill.name,
        limit: combatSkillLevel(character, skill.id),
        items: saved
          .filter((m) => m.skillId === skill.id)
          .map((m) => ({
            id: m.id,
            name: m.name,
            source: <span className="text-rose-300">{skill.name}</span>,
            bonus: maneuverBonus(character, skill.id),
            draft: m.draft,
          })),
      }))}
    />
  )
}
