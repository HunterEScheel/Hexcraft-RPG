import { useMemo, useState } from 'react'
import type { SavedSpell, SpellDraft } from '../system/spells'
import {
  MAGIC_MEDIUMS,
  MAGIC_SCHOOLS,
  type MagicMedium,
  type MagicSchool,
} from '../system/magicSchools'
import { PickerSelect } from './EffectBuilder'
import { QuickEffect } from './QuickEffect'

interface Props {
  schools: Record<MagicSchool, number>
  mediums: Record<MagicMedium, number>
  currentEp: number
  savedSpells: SavedSpell[]
  onCast: (epCost: number) => void
  onSave: (spell: {
    name: string
    school: MagicSchool
    medium: MagicMedium
    draft: SpellDraft
  }) => void
}

/** Build and cast a spell: a trained school and medium set its bonus. */
export function QuickCast({
  schools,
  mediums,
  currentEp,
  savedSpells,
  onCast,
  onSave,
}: Props) {
  const [school, setSchool] = useState<MagicSchool | ''>('')
  const [medium, setMedium] = useState<MagicMedium | ''>('')

  const trainedSchools = useMemo(
    () => MAGIC_SCHOOLS.filter((s) => schools[s] > 0),
    [schools],
  )
  const trainedMediums = useMemo(
    () => MAGIC_MEDIUMS.filter((m) => mediums[m] > 0),
    [mediums],
  )

  if (trainedSchools.length === 0 || trainedMediums.length === 0) {
    return (
      <p className="text-sm text-zinc-500 italic">
        Train at least one school and one medium to cast.
      </p>
    )
  }

  const picked = school !== '' && medium !== ''
  return (
    <QuickEffect
      noun="spell"
      verb="Cast"
      pickers={
        <div className="grid grid-cols-2 gap-3">
          <PickerSelect
            label="School"
            value={school}
            options={trainedSchools.map((s) => ({
              value: s,
              label: `${s} (Lv ${schools[s]})`,
            }))}
            onChange={(v) => setSchool(v as MagicSchool)}
          />
          <PickerSelect
            label="Medium"
            value={medium}
            options={trainedMediums.map((m) => ({
              value: m,
              label: `${m} (Lv ${mediums[m]})`,
            }))}
            onChange={(v) => setMedium(v as MagicMedium)}
          />
        </div>
      }
      pickerHint={school === '' ? 'Pick a school' : medium === '' ? 'Pick a medium' : null}
      bonus={picked ? schools[school] + mediums[medium] : null}
      currentEp={currentEp}
      slots={
        school === ''
          ? null
          : {
              label: school,
              used: savedSpells.filter((s) => s.school === school).length,
              limit: schools[school],
            }
      }
      onUse={onCast}
      onSave={(name, draft) =>
        onSave({ name, school: school as MagicSchool, medium: medium as MagicMedium, draft })
      }
    />
  )
}
