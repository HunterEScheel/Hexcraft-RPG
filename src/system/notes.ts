/** One of a character's notes on the sheet's Notes tab. */
export interface CharacterNote {
  id: string
  title: string
  body: string
  /** When it was last changed, as an ISO timestamp. */
  updatedAt: string
}

export function newNote(now = new Date()): CharacterNote {
  return { id: `note-${crypto.randomUUID()}`, title: '', body: '', updatedAt: now.toISOString() }
}

/**
 * Notes started as one block of text. A sheet saved that way keeps it as a
 * single note titled "Notes"; an empty one becomes no notes.
 */
export function migrateNotes(notes: unknown, now = new Date()): CharacterNote[] {
  if (Array.isArray(notes)) return notes as CharacterNote[]
  if (typeof notes === 'string' && notes.trim()) {
    return [{ ...newNote(now), title: 'Notes', body: notes }]
  }
  return []
}

/** "just now", "5 min ago", "3 h ago", then the date. */
export function editedLabel(iso: string, now = new Date()): string {
  const then = new Date(iso)
  const seconds = (now.getTime() - then.getTime()) / 1000
  if (Number.isNaN(seconds)) return ''
  if (seconds < 60) return 'just now'
  if (seconds < 3600) return `${Math.floor(seconds / 60)} min ago`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)} h ago`
  return then.toLocaleString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  })
}
