import { useState } from 'react'
import { counterspellChance, counterspellDC } from '../system/counterspell'
import { NumberStepper } from './NumberStepper'

/**
 * Whether a counterspell stops a spell: automatic with at least as much EP,
 * otherwise d20 + Destroy + Magic + spellcasting attribute against DC 10 +
 * every 10 EP of difference.
 */
export function CounterspellCalculator() {
  const [spellEp, setSpellEp] = useState(40)
  const [counterEp, setCounterEp] = useState(16)
  const [destroy, setDestroy] = useState(3)
  const [magic, setMagic] = useState(3)
  const [attribute, setAttribute] = useState(2)
  const bonus = destroy + magic + attribute

  const dc = counterspellDC(spellEp, counterEp)
  const chance = counterspellChance(bonus, dc)
  const needed = dc === null ? null : Math.max(2, dc - bonus)

  return (
    <section className="overflow-hidden rounded-lg border border-sky-700/40 bg-zinc-900/50">
      <header className="flex items-baseline justify-between border-b border-zinc-800 bg-zinc-950/50 px-4 py-2">
        <h3 className="text-[10px] font-medium uppercase tracking-[0.3em] text-sky-300/80">
          Counterspell
        </h3>
        <span className="text-[10px] uppercase tracking-[0.2em] text-zinc-500">Reaction</span>
      </header>

      <div className="space-y-4 p-4">
        <div className="flex flex-wrap gap-x-8 gap-y-2">
          <NumberStepper label="Spell's EP" value={spellEp} onChange={setSpellEp} min={0} />
          <NumberStepper label="Counter's EP" value={counterEp} onChange={setCounterEp} min={0} />
          <NumberStepper label="Destroy" value={destroy} onChange={setDestroy} min={0} />
          <NumberStepper label="Magic" value={magic} onChange={setMagic} min={0} />
          <NumberStepper label="Spellcasting attribute" value={attribute} onChange={setAttribute} min={-5} />
        </div>

        <div className="grid grid-cols-3 gap-3 rounded border border-zinc-800 bg-zinc-950 p-3 text-center">
          <Result
            label="DC"
            value={dc === null ? 'Auto' : dc}
            note={dc === null ? 'counter has the EP' : `10 + ${spellEp - counterEp} ÷ 10`}
          />
          <Result
            label="Needs"
            value={needed === null ? '—' : needed > 20 ? '—' : `${needed}+`}
            note={needed !== null && needed > 20 ? 'impossible' : 'on the d20'}
          />
          <Result label="Chance" value={`${Math.round(chance * 100)}%`} note={`d20 + ${bonus} · natural 1 fails`} />
        </div>
        <p className="text-xs text-zinc-500">
          A counter is a Destroy / Magic spell at Reaction timing. EP is what each caster
          actually spent, after casting time and amping. Either way the countered caster&apos;s
          EP is spent.
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
