import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  deleteCharacter,
  knownPasscode,
  listCharacters,
  type SavedCharacterRow,
} from '../lib/characters'
import { supabaseConfigured } from '../lib/supabase'
import { useIsAdmin, useUser } from '../lib/useUser'
import { bpBreakdown } from '../system/character'

export function Home() {
  const [rows, setRows] = useState<SavedCharacterRow[]>([])
  const [loading, setLoading] = useState(true)
  const user = useUser()
  const isAdmin = useIsAdmin(user)

  // Reload once the admin is known: the roster then opens every character.
  useEffect(() => {
    if (!supabaseConfigured) {
      setLoading(false)
      return
    }
    listCharacters().then((r) => {
      setRows(r)
      setLoading(false)
    })
  }, [isAdmin])

  // Who may delete: the admin always; a locked character, whoever holds its
  // passcode; an open one, anyone signed in. The database checks the same.
  const canDelete = (row: SavedCharacterRow) =>
    isAdmin || (row.locked ? knownPasscode(row.id) !== null : user !== null)

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this character?')) return
    const ok = await deleteCharacter(id)
    if (ok) setRows((rs) => rs.filter((r) => r.id !== id))
  }

  if (!supabaseConfigured) {
    return (
      <div className="rounded border border-amber-700/50 bg-amber-900/20 p-4 text-sm text-amber-200">
        Supabase isn&apos;t configured. Set <code>VITE_SUPABASE_URL</code> and{' '}
        <code>VITE_SUPABASE_ANON_KEY</code> in <code>.env</code> to save and load
        characters.
        <div className="mt-3">
          <Link
            to="/builder"
            className="inline-block rounded bg-amber-500 hover:bg-amber-400 px-3 py-1.5 font-medium text-zinc-950"
          >
            Try the builder anyway
          </Link>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">Characters</h2>
      </div>
      {loading ? (
        <p className="text-sm text-zinc-500">Loading…</p>
      ) : rows.length === 0 ? (
        <div className="rounded border border-zinc-800 bg-zinc-900/50 p-6 text-center">
          <p className="text-zinc-400">No characters yet.</p>
          <Link
            to="/builder"
            className="inline-block mt-3 rounded bg-amber-500 hover:bg-amber-400 px-3 py-1.5 font-medium text-zinc-950"
          >
            Create your first
          </Link>
        </div>
      ) : (
        <ul className="grid sm:grid-cols-2 gap-3">
          {rows.map((row) => (
            <li
              key={row.id}
              className="rounded border border-zinc-800 bg-zinc-900/50 p-4"
            >
              <div className="flex items-baseline justify-between mb-1">
                <h3 className="text-base font-medium text-zinc-100">
                  {row.locked && <span title="Locked with a passcode">🔒 </span>}
                  {row.name}
                </h3>
                {row.data && (
                  <span className="text-xs text-zinc-500 font-mono">
                    {row.data.tierName}
                  </span>
                )}
              </div>
              <p className="text-xs text-zinc-500 mb-3">
                {row.data
                  ? `${bpBreakdown(row.data).effectiveBudget} BP · HP ${row.data.hp} · EP ${row.data.ep}`
                  : 'Locked — open it with its passcode.'}
                {row.passcode && (
                  <span className="ml-2 font-mono text-amber-300/80">
                    Passcode: {row.passcode}
                  </span>
                )}
              </p>
              <div className="flex gap-2">
                <Link
                  to={`/sheet/${row.id}`}
                  className="flex-1 text-center rounded bg-zinc-800 hover:bg-zinc-700 px-3 py-1.5 text-sm"
                >
                  {row.data ? 'Sheet' : 'Unlock'}
                </Link>
                {row.data && (
                  <Link
                    to={`/builder/${row.id}`}
                    className="flex-1 text-center rounded bg-zinc-800 hover:bg-zinc-700 px-3 py-1.5 text-sm"
                  >
                    Edit
                  </Link>
                )}
                {canDelete(row) && (
                  <button
                    type="button"
                    onClick={() => handleDelete(row.id)}
                    className="rounded bg-zinc-800 hover:bg-rose-900/60 px-3 py-1.5 text-sm text-zinc-400 hover:text-rose-300"
                    aria-label="Delete"
                  >
                    ✕
                  </button>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
