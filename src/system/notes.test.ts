import { describe, expect, it } from 'vitest'
import { editedLabel, migrateNotes } from './notes'

describe('notes', () => {
  const now = new Date('2026-10-19T12:00:00Z')

  it('turns the old single block of text into one note', () => {
    const notes = migrateNotes('Owe Garrick 5 gold', now)
    expect(notes).toHaveLength(1)
    expect(notes[0]).toMatchObject({ title: 'Notes', body: 'Owe Garrick 5 gold', updatedAt: now.toISOString() })
    expect(migrateNotes('   ', now)).toEqual([])
    expect(migrateNotes(undefined, now)).toEqual([])
  })

  it('keeps notes that are already a list', () => {
    const list = [{ id: 'n', title: 'Leads', body: '', updatedAt: now.toISOString() }]
    expect(migrateNotes(list, now)).toBe(list)
  })

  it('labels how long ago a note changed', () => {
    expect(editedLabel('2026-10-19T11:59:30Z', now)).toBe('just now')
    expect(editedLabel('2026-10-19T11:55:00Z', now)).toBe('5 min ago')
    expect(editedLabel('2026-10-19T09:00:00Z', now)).toBe('3 h ago')
  })
})
