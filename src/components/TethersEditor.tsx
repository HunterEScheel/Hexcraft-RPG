import {
  TETHER_BP_BY_TIER,
  TETHER_TIERS,
  type Tether,
  type TetherTier,
} from '../system/tethers'

interface Props {
  value: Tether[]
  onChange: (next: Tether[]) => void
}

export function TethersEditor({ value, onChange }: Props) {
  // A blank tether, or one started from an example, which names it; the
  // player makes the title specific and writes the details.
  const add = (tier: TetherTier = 1, title = '') =>
    onChange([
      ...value,
      {
        id: `tether-${crypto.randomUUID()}`,
        title,
        description: '',
        tier,
      },
    ])

  const update = (id: string, patch: Partial<Tether>) =>
    onChange(value.map((t) => (t.id === id ? { ...t, ...patch } : t)))

  const remove = (id: string) =>
    onChange(value.filter((t) => t.id !== id))

  return (
    <div className="space-y-3">
      <div className="rounded border border-zinc-800 bg-zinc-950 p-3 space-y-2">
        <p className="text-xs text-zinc-500">
          Tethers are obligations the world will hold you to. Pick one to start from:
        </p>
        {TETHER_TIERS.map((opt) => (
          <div key={opt.tier} className="flex flex-wrap items-center gap-1">
            <span className="w-24 text-xs text-zinc-400">
              {opt.label}{' '}
              <span className="font-mono text-[10px] text-zinc-600">+{opt.bpRefund}</span>
            </span>
            {opt.examples.map((ex) => (
              <button
                key={ex}
                type="button"
                onClick={() => add(opt.tier as TetherTier, ex)}
                className="rounded border border-zinc-700 bg-zinc-900 px-2 py-0.5 text-xs text-zinc-300 hover:border-zinc-500"
              >
                {ex}
              </button>
            ))}
          </div>
        ))}
      </div>
      {value.length === 0 ? (
        <p className="text-sm text-zinc-500 italic">
          No tethers. Add one to claim BP back in exchange for an obligation.
        </p>
      ) : (
        <ul className="space-y-2">
          {value.map((t) => (
            <li
              key={t.id}
              className="rounded border border-zinc-800 bg-zinc-900 p-3 space-y-2"
            >
              <div className="flex items-start gap-2">
                <div className="flex-1 space-y-2">
                  <input
                    type="text"
                    value={t.title}
                    onChange={(e) => update(t.id, { title: e.target.value })}
                    placeholder="Title, e.g. Sworn oath to Prince Aldric"
                    aria-label="Tether title"
                    className="w-full bg-zinc-950 border border-zinc-700 rounded px-3 py-1.5 text-sm text-zinc-100 placeholder:text-zinc-500"
                  />
                  <textarea
                    value={t.description}
                    onChange={(e) =>
                      update(t.id, { description: e.target.value })
                    }
                    placeholder="Description: who holds you to it, what it demands, what happens if you break it"
                    aria-label="Tether description"
                    rows={2}
                    className="w-full bg-zinc-950 border border-zinc-700 rounded px-3 py-2 text-sm text-zinc-100 placeholder:text-zinc-500 resize-y"
                  />
                </div>
                <button
                  type="button"
                  onClick={() => remove(t.id)}
                  aria-label="Remove tether"
                  className="text-zinc-500 hover:text-rose-400 text-sm px-1"
                >
                  ✕
                </button>
              </div>
              <div className="flex gap-2">
                {TETHER_TIERS.map((opt) => {
                  const active = opt.tier === t.tier
                  return (
                    <button
                      key={opt.tier}
                      type="button"
                      onClick={() =>
                        update(t.id, { tier: opt.tier as TetherTier })
                      }
                      className={
                        'flex-1 rounded border px-2 py-1 text-xs ' +
                        (active
                          ? 'border-emerald-500 bg-emerald-900/30 text-emerald-200'
                          : 'border-zinc-700 bg-zinc-950 text-zinc-400 hover:border-zinc-500')
                      }
                    >
                      <div className="font-medium">{opt.label}</div>
                      <div className="font-mono text-[10px] text-zinc-500">
                        +{TETHER_BP_BY_TIER[opt.tier as TetherTier]} BP · w{opt.tier}
                      </div>
                    </button>
                  )
                })}
              </div>
            </li>
          ))}
        </ul>
      )}
      <button
        type="button"
        onClick={() => add()}
        className="rounded bg-zinc-800 hover:bg-zinc-700 px-3 py-1.5 text-sm text-zinc-200"
      >
        + Add tether
      </button>
    </div>
  )
}
