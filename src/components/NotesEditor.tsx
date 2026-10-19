import { useEffect, useState } from 'react'
import { editedLabel, newNote, type CharacterNote } from '../system/notes'

/**
 * A character's notes: add as many as you like, each with a title, a body and
 * when it was last changed. Newest first.
 */
export function NotesEditor({
  notes,
  onChange,
}: {
  notes: CharacterNote[]
  onChange: (next: CharacterNote[]) => void
}) {
  // Re-render each minute so "5 min ago" stays true.
  const [now, setNow] = useState(() => new Date())
  useEffect(() => {
    const t = window.setInterval(() => setNow(new Date()), 60_000)
    return () => window.clearInterval(t)
  }, [])
  const [confirming, setConfirming] = useState<string | null>(null)

  const update = (id: string, patch: Partial<CharacterNote>) =>
    onChange(
      notes.map((n) =>
        n.id === id ? { ...n, ...patch, updatedAt: new Date().toISOString() } : n,
      ),
    )

  return (
    <div className="space-y-3">
      <button
        type="button"
        onClick={() => onChange([newNote(), ...notes])}
        className="rounded bg-amber-500 hover:bg-amber-400 px-3 py-1.5 text-sm font-medium text-zinc-950"
      >
        + Add note
      </button>

      {notes.length === 0 && (
        <p className="text-sm text-zinc-500 italic">
          No notes yet. Add one for NPCs met, leads, debts owed, loot to split…
        </p>
      )}

      <ul className="space-y-3">
        {notes.map((n) => (
          <li key={n.id} className="rounded border border-zinc-800 bg-zinc-900 p-3 space-y-2">
            <div className="flex items-center gap-2">
              <input
                type="text"
                value={n.title}
                onChange={(e) => update(n.id, { title: e.target.value })}
                placeholder="Title"
                aria-label="Note title"
                className="flex-1 min-w-0 bg-zinc-950 border border-zinc-700 rounded px-2 py-1.5 text-sm font-medium text-zinc-100 placeholder:text-zinc-600"
              />
              {confirming === n.id ? (
                <span className="flex items-center gap-2 text-xs">
                  <span className="text-zinc-400">Delete?</span>
                  <button
                    type="button"
                    onClick={() => {
                      onChange(notes.filter((x) => x.id !== n.id))
                      setConfirming(null)
                    }}
                    className="rounded bg-rose-600 hover:bg-rose-500 px-2 py-1 font-medium text-zinc-950"
                  >
                    Delete
                  </button>
                  <button
                    type="button"
                    onClick={() => setConfirming(null)}
                    className="text-zinc-400 hover:text-zinc-200"
                  >
                    Cancel
                  </button>
                </span>
              ) : (
                <button
                  type="button"
                  onClick={() => setConfirming(n.id)}
                  aria-label={`Delete ${n.title || 'note'}`}
                  className="text-zinc-500 hover:text-rose-400 text-sm px-1"
                >
                  ✕
                </button>
              )}
            </div>
            <textarea
              value={n.body}
              onChange={(e) => update(n.id, { body: e.target.value })}
              placeholder="Write anything…"
              aria-label="Note"
              rows={4}
              className="w-full bg-zinc-950 border border-zinc-800 rounded px-3 py-2 text-sm leading-relaxed text-zinc-100 placeholder:text-zinc-600 resize-y"
            />
            <div
              className="text-[10px] text-zinc-500"
              title={new Date(n.updatedAt).toLocaleString()}
            >
              Last changed {editedLabel(n.updatedAt, now)}
            </div>
          </li>
        ))}
      </ul>
    </div>
  )
}
