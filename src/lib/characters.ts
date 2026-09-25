import type { Character } from '../system/character'
import { supabase, supabaseConfigured } from './supabase'

/**
 * Characters are read and written only through the hexcraft_* functions in the
 * jaeg.click repo's hexcraft_0003 migration, which enforce the optional
 * passcode: an open character is anyone's to view and edit, a locked one only
 * its passcode holder's or the site admin's.
 */
export interface SavedCharacterRow {
  id: string
  name: string
  /** Null when the character is locked and this browser can't open it. */
  data: Character | null
  locked: boolean
  /** Returned to the admin, and to whoever holds it on a single fetch. */
  passcode: string | null
  created_at: string
  updated_at: string
}

// Passcodes this browser has been given, so a locked character stays open
// across pages and visits without asking again.
const PASSCODES_KEY = 'hexcraft-passcodes'

function passcodes(): Record<string, string> {
  try {
    return JSON.parse(localStorage.getItem(PASSCODES_KEY) ?? '{}')
  } catch {
    return {}
  }
}

export function knownPasscode(id: string): string | null {
  return passcodes()[id] ?? null
}

export function rememberPasscode(id: string, passcode: string | null) {
  const all = passcodes()
  if (passcode) all[id] = passcode
  else delete all[id]
  try {
    localStorage.setItem(PASSCODES_KEY, JSON.stringify(all))
  } catch {
    // Storage unavailable: the passcode is asked for again next time.
  }
}

export async function saveCharacter(
  c: Character,
  passcode: string | null = null,
): Promise<string | null> {
  if (!supabaseConfigured || !supabase) {
    console.warn('supabase not configured — character not saved')
    return null
  }
  const { data, error } = await supabase.rpc('hexcraft_create_character', {
    p_name: c.name || 'Unnamed',
    p_data: c,
    p_passcode: passcode || null,
  })
  if (error) {
    console.error('saveCharacter failed', error)
    return null
  }
  const id = data as string
  if (passcode) rememberPasscode(id, passcode)
  return id
}

export async function updateCharacter(
  id: string,
  c: Character,
): Promise<boolean> {
  if (!supabaseConfigured || !supabase) return false
  const { data, error } = await supabase.rpc('hexcraft_update_character', {
    p_id: id,
    p_passcode: knownPasscode(id),
    p_name: c.name || 'Unnamed',
    p_data: c,
  })
  if (error) {
    console.error('updateCharacter failed', error)
    return false
  }
  return data === true
}

/** Change or remove (null) a locked character's passcode. */
export async function setPasscode(
  id: string,
  next: string | null,
): Promise<boolean> {
  if (!supabaseConfigured || !supabase) return false
  const { data, error } = await supabase.rpc('hexcraft_set_passcode', {
    p_id: id,
    p_passcode: knownPasscode(id),
    p_new_passcode: next || null,
  })
  if (error) {
    console.error('setPasscode failed', error)
    return false
  }
  if (data === true) rememberPasscode(id, next || null)
  return data === true
}

/**
 * One character, opened with `passcode` or the one this browser remembers.
 * A locked character comes back with `data: null` when the passcode is wrong.
 */
export async function getCharacter(
  id: string,
  passcode: string | null = knownPasscode(id),
): Promise<SavedCharacterRow | null> {
  if (!supabaseConfigured || !supabase) return null
  const { data, error } = await supabase
    .rpc('hexcraft_get_character', { p_id: id, p_passcode: passcode })
    .maybeSingle()
  if (error) {
    console.error('getCharacter failed', error)
    return null
  }
  return (data as SavedCharacterRow | null) ?? null
}

export async function listCharacters(): Promise<SavedCharacterRow[]> {
  if (!supabaseConfigured || !supabase) return []
  const { data, error } = await supabase.rpc('hexcraft_list_characters', {
    p_passcodes: passcodes(),
  })
  if (error) {
    console.error('listCharacters failed', error)
    return []
  }
  return (data ?? []) as SavedCharacterRow[]
}

export async function deleteCharacter(id: string): Promise<boolean> {
  if (!supabaseConfigured || !supabase) return false
  const { data, error } = await supabase.rpc('hexcraft_delete_character', {
    p_id: id,
    p_passcode: knownPasscode(id),
  })
  if (error) {
    console.error('deleteCharacter failed', error)
    return false
  }
  if (data === true) rememberPasscode(id, null)
  return data === true
}
