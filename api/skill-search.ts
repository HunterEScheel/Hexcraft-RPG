// Skill search by meaning: turns the search text into an OpenAI embedding and
// asks Supabase for the closest skills (hexcraft_match_skills). Runs on Vercel
// so the OpenAI key never reaches the browser. Needs OPENAI_API_KEY in the
// Vercel project's environment variables; the Supabase URL and anon key are the
// ones the app already uses.
//
// GET /api/skill-search?q=ride+a+horse → [{ id, name, description, similarity }]

const MODEL = process.env.EMBEDDING_MODEL || 'text-embedding-3-small'
const MAX_QUERY = 80
const MATCHES = 8

export async function GET(request: Request): Promise<Response> {
  const q = (new URL(request.url).searchParams.get('q') ?? '').trim().toLowerCase()
  if (q.length < 3 || q.length > MAX_QUERY) return json([])

  const openaiKey = process.env.OPENAI_API_KEY
  const supabaseUrl = process.env.SUPABASE_URL || process.env.VITE_SUPABASE_URL
  const anonKey = process.env.SUPABASE_ANON_KEY || process.env.VITE_SUPABASE_ANON_KEY
  if (!openaiKey || !supabaseUrl || !anonKey) {
    return json({ error: 'Skill search by meaning is not configured' }, 503)
  }

  const embedRes = await fetch('https://api.openai.com/v1/embeddings', {
    method: 'POST',
    headers: { 'content-type': 'application/json', authorization: `Bearer ${openaiKey}` },
    body: JSON.stringify({ model: MODEL, input: q }),
  })
  if (!embedRes.ok) {
    console.error('OpenAI', embedRes.status, await embedRes.text())
    return json({ error: 'Embedding failed' }, 502)
  }
  const embedding = ((await embedRes.json()) as { data: { embedding: number[] }[] }).data[0]
    .embedding

  const matchRes = await fetch(`${supabaseUrl}/rest/v1/rpc/hexcraft_match_skills`, {
    method: 'POST',
    headers: {
      'content-type': 'application/json',
      apikey: anonKey,
      authorization: `Bearer ${anonKey}`,
    },
    body: JSON.stringify({ query_embedding: embedding, match_count: MATCHES }),
  })
  if (!matchRes.ok) {
    console.error('Supabase', matchRes.status, await matchRes.text())
    return json({ error: 'Skill lookup failed' }, 502)
  }
  // The same words give the same answer, so let the CDN keep it for a day:
  // repeat searches cost nothing.
  return json(await matchRes.json(), 200, 'public, s-maxage=86400, max-age=300')
}

function json(body: unknown, status = 200, cache = 'no-store'): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json', 'cache-control': cache },
  })
}
