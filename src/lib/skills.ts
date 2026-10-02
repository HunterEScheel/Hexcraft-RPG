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
  /** A suggested specificity, e.g. "Dogs" for Animal Handling. */
  specificity: string | null
  description: string | null
  similarity?: number
}

// Matches the name or the specificity, so "dogs" finds Animal Handling (Dogs).
// "Name (Specificity)" narrows to both.
export async function searchSkills(
  query: string,
  limit = 20,
): Promise<SkillSearchResult[]> {
  if (!supabaseConfigured || !supabase || !query.trim()) return []

  // PostgREST filter syntax uses commas and parentheses.
  const clean = (t: string) => t.replace(/[,()*%\\]/g, ' ').trim()
  const { name, specificity } = splitSkillName(query)
  let request = supabase
    .from('hexcraft_skill_embeddings')
    .select('id, skill_name, specificity, description')
  request = specificity
    ? request.ilike('skill_name', `%${clean(name)}%`).ilike('specificity', `%${clean(specificity)}%`)
    : request.or(`skill_name.ilike.%${clean(name)}%,specificity.ilike.%${clean(name)}%`)
  const { data, error } = await request
    .order('skill_name', { ascending: true })
    .order('specificity', { ascending: true, nullsFirst: true })
    .limit(limit)

  if (error) {
    console.error('skill search failed', error)
    return []
  }

  return (data ?? []).map((row) => ({
    id: row.id as number,
    name: row.skill_name as string,
    specificity: (row.specificity as string | null) ?? null,
    description: (row.description as string | null) ?? null,
  }))
}

export async function searchSkillsBySimilarity(
  embedding: number[],
  limit = 12,
): Promise<SkillSearchResult[]> {
  if (!supabaseConfigured || !supabase) return []

  const { data, error } = await supabase.rpc('hexcraft_match_skills', {
    query_embedding: embedding,
    match_count: limit,
  })

  if (error) {
    console.error('similarity search failed', error)
    return []
  }

  return (data ?? []) as SkillSearchResult[]
}
