// "Run embeddings" for the site admin: embeds the skill list with OpenAI so
// skill search can find skills by meaning (see skill-search.ts). Called from the
// Tools page with the admin's Supabase session; the embeddings are saved through
// hexcraft_set_skill_embeddings, which checks is_site_admin() itself, so no
// service-role key lives here. Needs OPENAI_API_KEY in Vercel's environment.
//
// POST /api/run-embeddings  { all?: boolean }  → { embedded, total }
// Without `all`, only skills that have no embedding yet are embedded.

export const maxDuration = 60

const MODEL = process.env.EMBEDDING_MODEL || 'text-embedding-3-small'
const EMBED_BATCH = 256
const SAVE_BATCH = 50

export async function POST(request: Request): Promise<Response> {
  const openaiKey = process.env.OPENAI_API_KEY
  const supabaseUrl = process.env.SUPABASE_URL || process.env.VITE_SUPABASE_URL
  const anonKey = process.env.SUPABASE_ANON_KEY || process.env.VITE_SUPABASE_ANON_KEY
  if (!openaiKey || !supabaseUrl || !anonKey) {
    return json({ error: 'OPENAI_API_KEY is not set in Vercel' }, 503)
  }
  const token = request.headers.get('authorization')?.replace(/^Bearer\s+/i, '')
  if (!token) return json({ error: 'Sign in first' }, 401)

  // Everything below runs as the signed-in user.
  const db = (path: string, init: RequestInit = {}) =>
    fetch(`${supabaseUrl}/rest/v1/${path}`, {
      ...init,
      headers: {
        'content-type': 'application/json',
        apikey: anonKey,
        authorization: `Bearer ${token}`,
        ...init.headers,
      },
    })

  // Checked again by the database; this just fails fast for non-admins.
  const adminRes = await db('rpc/is_site_admin', { method: 'POST', body: '{}' })
  if (!adminRes.ok || (await adminRes.json()) !== true) {
    return json({ error: 'Only the site admin can run embeddings' }, 403)
  }

  const body = (await request.json().catch(() => ({}))) as { all?: boolean }
  const filter = body.all ? '' : '&embedding=is.null'
  const listRes = await db(
    `hexcraft_skill_embeddings?select=id,skill_name,description${filter}&order=skill_name`,
  )
  if (!listRes.ok) return json({ error: `Reading skills failed: ${await listRes.text()}` }, 502)
  const skills = (await listRes.json()) as {
    id: number
    skill_name: string
    description: string | null
  }[]

  let embedded = 0
  for (let i = 0; i < skills.length; i += EMBED_BATCH) {
    const batch = skills.slice(i, i + EMBED_BATCH)
    const embedRes = await fetch('https://api.openai.com/v1/embeddings', {
      method: 'POST',
      headers: { 'content-type': 'application/json', authorization: `Bearer ${openaiKey}` },
      body: JSON.stringify({
        model: MODEL,
        input: batch.map((s) => (s.description ? `${s.skill_name}: ${s.description}` : s.skill_name)),
      }),
    })
    if (!embedRes.ok) {
      return json({ error: `OpenAI ${embedRes.status}: ${await embedRes.text()}`, embedded }, 502)
    }
    const vectors = ((await embedRes.json()) as { data: { embedding: number[] }[] }).data

    for (let j = 0; j < batch.length; j += SAVE_BATCH) {
      const rows = batch
        .slice(j, j + SAVE_BATCH)
        .map((s, k) => ({ id: s.id, embedding: vectors[j + k].embedding }))
      const saveRes = await db('rpc/hexcraft_set_skill_embeddings', {
        method: 'POST',
        body: JSON.stringify({ p_rows: rows }),
      })
      if (!saveRes.ok) {
        return json({ error: `Saving failed: ${await saveRes.text()}`, embedded }, 502)
      }
      embedded += (await saveRes.json()) as number
    }
  }

  return json({ embedded, total: skills.length })
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json', 'cache-control': 'no-store' },
  })
}
