import { useMemo, useState } from 'react'
import { emptySpellDraft, spellCost } from '../system/spells'
import { spellTargetingLabel, type MonsterSpell } from '../system/monster'
import { EffectBuilder } from './EffectBuilder'

interface Props {
  spellBonus: number
  value: MonsterSpell[]
  onChange: (next: MonsterSpell[]) => void
}

export function MonsterSpellComposer({ spellBonus, value, onChange }: Props) {
  const [draft, setDraft] = useState(emptySpellDraft)
  const [name, setName] = useState('')

  const cost = useMemo(() => spellCost(draft), [draft])

  const canAdd = name.trim().length > 0 && cost.totalEp > 0

  const handleAdd = () => {
    if (!canAdd) return
    onChange([
      ...value,
      {
        id: `monster-spell-${crypto.randomUUID()}`,
        name: name.trim(),
        draft,
      },
    ])
    setName('')
    setDraft(emptySpellDraft())
  }

  return (
    <div className="space-y-4">
      <EffectBuilder draft={draft} onChange={setDraft} />

      <div className="rounded border border-zinc-800 bg-zinc-950 p-3 flex items-center gap-2 flex-wrap">
        <span className="text-xs text-zinc-500 font-mono whitespace-nowrap">
          {cost.baseEp} base × {cost.multiplier} = {cost.totalEp} EP
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
          disabled={!canAdd}
          title={
            name.trim().length === 0
              ? 'Give the spell a name'
              : cost.totalEp === 0
                ? 'Set at least one criterion or damage die'
                : undefined
          }
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
              <span className="text-sm text-zinc-100 whitespace-nowrap">
                {s.name}
              </span>
              <span className="text-xs font-mono text-amber-300 whitespace-nowrap">
                {spellTargetingLabel(s, spellBonus)}
              </span>
              <span className="text-xs font-mono text-zinc-500 flex-1">
                {spellCost(s.draft).totalEp} EP
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
