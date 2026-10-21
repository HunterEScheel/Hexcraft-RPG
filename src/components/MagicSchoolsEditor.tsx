import { MAGIC_SCHOOLS, type MagicSchool } from '../system/magicSchools'
import { NumberStepper } from './NumberStepper'

interface Props {
  value: Record<MagicSchool, number>
  onChange: (next: Record<MagicSchool, number>) => void
  /** Characters stop at 5; monsters can go higher. */
  max?: number
}

export function MagicSchoolsEditor({ value, onChange, max = 5 }: Props) {
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
      {MAGIC_SCHOOLS.map((school) => (
        <NumberStepper
          key={school}
          label={school}
          value={value[school]}
          onChange={(v) => onChange({ ...value, [school]: v })}
          min={0}
          max={max}
        />
      ))}
    </div>
  )
}
