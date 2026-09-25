import { useState } from 'react'
import { Link } from 'react-router-dom'
import {
  getCharacter,
  rememberPasscode,
  type SavedCharacterRow,
} from '../lib/characters'

/**
 * Shown in place of a locked character's sheet or builder. The passcode is
 * checked by fetching the character with it; a match is remembered in this
 * browser and hands back the opened row.
 */
export function UnlockForm({
  id,
  name,
  onUnlock,
}: {
  id: string
  name: string
  onUnlock: (row: SavedCharacterRow) => void
}) {
  const [passcode, setPasscode] = useState('')
  const [checking, setChecking] = useState(false)
  const [wrong, setWrong] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setChecking(true)
    setWrong(false)
    const row = await getCharacter(id, passcode)
    setChecking(false)
    if (row?.data) {
      rememberPasscode(id, passcode)
      onUnlock(row)
    } else {
      setWrong(true)
    }
  }

  return (
    <form
      onSubmit={submit}
      className="mx-auto max-w-sm space-y-3 rounded border border-zinc-800 bg-zinc-900/50 p-5"
    >
      <h2 className="text-base font-medium">🔒 {name}</h2>
      <p className="text-sm text-zinc-400">
        This character is locked. Enter its passcode to open it.
      </p>
      <input
        type="password"
        value={passcode}
        onChange={(e) => setPasscode(e.target.value)}
        autoFocus
        required
        placeholder="Passcode"
        className="w-full bg-zinc-900 border border-zinc-700 rounded px-3 py-2 text-zinc-100"
      />
      {wrong && <p className="text-sm text-rose-400">That passcode doesn&apos;t match.</p>}
      <div className="flex items-center justify-between">
        <Link to="/" className="text-sm text-zinc-500 hover:text-zinc-300">
          ← Back to roster
        </Link>
        <button
          type="submit"
          disabled={checking}
          className="rounded bg-amber-500 hover:bg-amber-400 px-3 py-1.5 font-medium text-zinc-950 disabled:opacity-50"
        >
          {checking ? 'Checking…' : 'Unlock'}
        </button>
      </div>
    </form>
  )
}
