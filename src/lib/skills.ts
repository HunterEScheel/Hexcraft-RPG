import { supabase, supabaseConfigured } from './supabase'
import { splitSkillName } from '../system/character'

export interface SkillEmbedding {
  id: number
  name: string
  description: string | null
  embedding: number[]
}

export interface SkillSearchResult {
  id: number
  name: string
  description: string | null
  similarity?: number
}

// Skills are listed once each; a specificity is the character's own, so
// "Driving (Motorcycles)" searches for Driving.
export async function searchSkills(
  query: string,
  limit = 20,
): Promise<SkillSearchResult[]> {
  if (!supabaseConfigured || !supabase || !query.trim()) return []

  // PostgREST filter syntax uses commas and parentheses.
  const clean = (t: string) => t.replace(/[,()*%\\]/g, ' ').trim()
  const { name } = splitSkillName(query)
  const { data, error } = await supabase
    .from('hexcraft_skill_embeddings')
    .select('id, skill_name, description')
    .ilike('skill_name', `%${clean(name)}%`)
    .order('skill_name', { ascending: true })
    .limit(limit)

  if (error) {
    console.error('skill search failed', error)
    return []
  }

  return (data ?? []).map((row) => ({
    id: row.id as number,
    name: row.skill_name as string,
    description: (row.description as string | null) ?? null,
  }))
}

// Skills that mean what was typed ("ride a horse" → Riding), from the
// /api/skill-search function, which holds the OpenAI key. Empty when that
// isn't set up or the skills have no embeddings yet.
export async function searchSkillsByMeaning(
  query: string,
  signal?: AbortSignal,
): Promise<SkillSearchResult[]> {
  const q = splitSkillName(query).name.trim().toLowerCase()
  if (q.length < 3) return []
  try {
    const res = await fetch(`/api/skill-search?q=${encodeURIComponent(q)}`, { signal })
    if (!res.ok) return []
    const data: unknown = await res.json()
    return Array.isArray(data) ? (data as SkillSearchResult[]) : []
  } catch {
    return []
  }
}

// A custom skill a player adds joins the shared list (hexcraft_add_skill), so
// everyone's search finds it. A name already listed is left alone. Failing
// quietly is fine: the skill is on the character either way.
export async function addSkillToList(name: string): Promise<void> {
  if (!supabaseConfigured || !supabase || !name.trim()) return
  const { error } = await supabase.rpc('hexcraft_add_skill', { p_name: name.trim() })
  if (error) console.error('adding skill to the list failed', error)
}
