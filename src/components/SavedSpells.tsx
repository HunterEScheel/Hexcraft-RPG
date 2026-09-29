import type { SavedSpell } from '../system/spells'
import {
  MAGIC_SCHOOLS,
  type MagicMedium,
  type MagicSchool,
} from '../system/magicSchools'
import { SavedEffects } from './SavedEffects'

interface Props {
  schools: Record<MagicSchool, number>
  mediums: Record<MagicMedium, number>
  savedSpells: SavedSpell[]
  currentEp: number
  onCast: (epCost: number) => void
  onRemove: (id: string) => void
}

export function SavedSpells({
  schools,
  mediums,
  savedSpells,
  currentEp,
  onCast,
  onRemove,
}: Props) {
  const trainedSchools = MAGIC_SCHOOLS.filter((s) => schools[s] > 0)
  if (trainedSchools.length === 0) return null

  if (savedSpells.length === 0) {
    return (
      <p className="text-sm text-zinc-500 italic">
        No saved spells yet. Build one above and click Save spell to prepare it
        at a 25% cost reduction.
      </p>
    )
  }

  return (
    <SavedEffects
      verb="Cast"
      currentEp={currentEp}
      onUse={onCast}
      onRemove={onRemove}
      groups={trainedSchools.map((school) => ({
        key: school,
        label: school,
        limit: schools[school],
        items: savedSpells
          .filter((s) => s.school === school)
          .map((s) => ({
            id: s.id,
            name: s.name,
            source: (
              <>
                <span className="text-violet-300">{s.school}</span>
                <span className="text-zinc-600 mx-1">·</span>
                <span className="text-sky-300">{s.medium}</span>
              </>
            ),
            bonus:
              ((schools as Record<string, number>)[s.school] ?? 0) +
              ((mediums as Record<string, number>)[s.medium] ?? 0),
            draft: s.draft,
          })),
      }))}
    />
  )
}
