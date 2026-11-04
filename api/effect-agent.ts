// "Describe it" for spells and maneuvers, site admin only: turns a plain-language
// description into picks from the builder's own options. The browser sends the
// description with a catalog of what the builder offers right now (see
// src/system/effectAgent.ts); this asks an OpenAI model to choose from it with a
// strict JSON schema built from that catalog, and returns the answer. The
// browser then checks every pick again before loading it into the builder.
// Runs on Vercel so the OpenAI key never reaches the browser.
//
// POST /api/effect-agent  { description, catalog }  → { output }
// Needs OPENAI_API_KEY in Vercel; EFFECT_AGENT_MODEL overrides the model.

// The catalog's shape: AgentCatalog in src/system/effectAgent.ts, repeated here
// because the functions are built apart from the app. Keep the two in step.
export interface AgentCatalog {
  kind: 'spell' | 'maneuver'
  criteria: { key: string; label: string; repeatable: boolean; choices: string[] }[]
  castingTimes: { key: string; label: string }[]
  targetings: { key: string; label: string }[]
}

export const maxDuration = 60

// Needs a model with strict JSON-schema output that the key's project can use.
const MODEL = process.env.EFFECT_AGENT_MODEL || 'gpt-4.1-mini'
const MAX_DESCRIPTION = 1000
const MAX_CHOICES = 200

export async function POST(request: Request): Promise<Response> {
  const openaiKey = process.env.OPENAI_API_KEY
  const supabaseUrl = process.env.SUPABASE_URL || process.env.VITE_SUPABASE_URL
  const anonKey = process.env.SUPABASE_ANON_KEY || process.env.VITE_SUPABASE_ANON_KEY
  if (!openaiKey || !supabaseUrl || !anonKey) {
    return json({ error: 'OPENAI_API_KEY is not set in Vercel' }, 503)
  }
  const token = request.headers.get('authorization')?.replace(/^Bearer\s+/i, '')
  if (!token) return json({ error: 'Sign in first' }, 401)

  // Each call costs OpenAI money, so only the site admin may make one.
  const adminRes = await fetch(`${supabaseUrl}/rest/v1/rpc/is_site_admin`, {
    method: 'POST',
    headers: {
      'content-type': 'application/json',
      apikey: anonKey,
      authorization: `Bearer ${token}`,
    },
    body: '{}',
  })
  if (!adminRes.ok || (await adminRes.json()) !== true) {
    return json({ error: 'Only the site admin can use the spell agent' }, 403)
  }

  const body = (await request.json().catch(() => null)) as {
    description?: unknown
    catalog?: unknown
  } | null
  const description = typeof body?.description === 'string' ? body.description.trim() : ''
  if (description.length < 3 || description.length > MAX_DESCRIPTION) {
    return json({ error: `Describe it in 3–${MAX_DESCRIPTION} characters` }, 400)
  }
  const catalog = parseCatalog(body?.catalog)
  if (!catalog) return json({ error: 'Bad catalog' }, 400)

  const res = await fetch('https://api.openai.com/v1/chat/completions', {
    method: 'POST',
    headers: { 'content-type': 'application/json', authorization: `Bearer ${openaiKey}` },
    body: JSON.stringify({
      model: MODEL,
      messages: [
        { role: 'system', content: systemPrompt(catalog) },
        { role: 'user', content: description },
      ],
      response_format: {
        type: 'json_schema',
        json_schema: { name: `hexcraft_${catalog.kind}`, strict: true, schema: responseSchema(catalog) },
      },
    }),
  })
  if (!res.ok) {
    const text = await res.text()
    console.error('OpenAI', res.status, text)
    // OpenAI's own reason ("model not found", a bad schema, out of quota), which
    // only the admin sees; it carries no secrets.
    let reason = ''
    try {
      reason = (JSON.parse(text) as { error?: { message?: string } }).error?.message ?? ''
    } catch {
      // Not JSON: the status alone will do.
    }
    return json({ error: `The model call failed (${res.status}, ${MODEL})${reason ? `: ${reason}` : ''}` }, 502)
  }
  const message = ((await res.json()) as {
    choices?: { message?: { content?: string | null; refusal?: string | null } }[]
  }).choices?.[0]?.message
  if (message?.refusal) return json({ error: `The model declined: ${message.refusal}` }, 422)
  try {
    return json({ output: JSON.parse(message?.content ?? '') })
  } catch {
    return json({ error: 'The model did not return JSON' }, 502)
  }
}

// The catalog comes from the browser, so check its shape and size before it
// goes into a prompt and a schema.
export function parseCatalog(raw: unknown): AgentCatalog | null {
  if (typeof raw !== 'object' || raw === null) return null
  const c = raw as Record<string, unknown>
  if (c.kind !== 'spell' && c.kind !== 'maneuver') return null
  const strings = (v: unknown, max: number) =>
    Array.isArray(v) && v.length <= max && v.every((s) => typeof s === 'string' && s.length <= 120)
  const keyed = (v: unknown) =>
    Array.isArray(v) &&
    v.length > 0 &&
    v.length <= 20 &&
    v.every((o) => typeof o?.key === 'string' && typeof o?.label === 'string')
  if (!keyed(c.castingTimes) || !keyed(c.targetings)) return null
  if (!Array.isArray(c.criteria) || c.criteria.length === 0 || c.criteria.length > 12) return null
  const criteriaOk = c.criteria.every(
    (k) =>
      typeof k?.key === 'string' &&
      /^[a-zA-Z]+$/.test(k.key) &&
      typeof k?.label === 'string' &&
      typeof k?.repeatable === 'boolean' &&
      strings(k?.choices, MAX_CHOICES) &&
      (k.repeatable || k.choices.length > 0),
  )
  return criteriaOk ? (raw as AgentCatalog) : null
}

const stringEnum = (values: string[], description?: string) => ({
  type: 'string',
  enum: values,
  ...(description ? { description } : {}),
})

// Strict mode: every property required, no extras, choices as enums, so the
// answer can only name options the builder has.
export function responseSchema(catalog: AgentCatalog) {
  const once = catalog.criteria.filter((c) => !c.repeatable)
  const many = catalog.criteria.filter((c) => c.repeatable)
  const object = (properties: Record<string, unknown>) => ({
    type: 'object',
    additionalProperties: false,
    required: Object.keys(properties),
    properties,
  })
  return object({
    name: { type: 'string', description: 'A short evocative name for it' },
    picks: object(Object.fromEntries(once.map((c) => [c.key, stringEnum(c.choices, c.label)]))),
    lists: object(
      Object.fromEntries(
        many.map((c) => [c.key, { type: 'array', description: c.label, items: stringEnum(c.choices) }]),
      ),
    ),
    damageDice: { type: 'integer', description: 'Number of d6s' },
    castingTime: stringEnum(catalog.castingTimes.map((t) => t.key)),
    targeting: stringEnum(catalog.targetings.map((t) => t.key)),
    notes: {
      type: 'string',
      description: 'Anything described that the options cannot express, or empty',
    },
  })
}

// The rules, condensed from the Players' Guide (scripts/players-guide/build.py).
// The option lists themselves are in the schema.
function systemPrompt(catalog: AgentCatalog): string {
  const timings = catalog.castingTimes.map((t) => `${t.key} = ${t.label}`).join(', ')
  const targetings = catalog.targetings.map((t) => `${t.key} = ${t.label}`).join(', ')
  const criteria = catalog.criteria
    .map((c) => `- ${c.key} (${c.label}${c.repeatable ? ', a list: pick any number, or none' : ''})`)
    .join('\n')
  const common = `You translate a player's plain-language description into picks for the Hexcraft RPG ${catalog.kind} builder. Pick exactly what the description asks for, no more: you are not optimizing cost, and unmentioned criteria take their cheapest choice (the first in each list, "none", or an empty list).

Criteria you fill in:
${criteria}

How to choose:
- Each criterion's choices run from smallest to largest. Pick the smallest one that covers what was described (a 60 ft range needs "120 ft"; a 20 ft radius is a 20 ft splash).
- damageDice counts d6s (damage, or healing dice). Convert other dice by average: NdX becomes N×(X+1)/7 d6s, rounded. Flat damage of D becomes D/3.5 d6s, rounded. No dice → 0.
- castingTime keys: ${timings}. "An action" is action, "two actions" 2actions, "a turn" or "a round" 1round, "a reaction" or "in response" reaction.
- targeting keys: ${targetings}. A bolt, strike or attack roll is hit. Something the target dodges out of (an explosion, a wave) is dodge. Something endured with the body (poison, disease, being knocked down) is grit. Something resisted with the mind (fear, charm, domination) is resolve. For a buff on yourself or willing allies, use hit.
- Buffs, debuffs and conditions must be named in the description or clearly implied. If one the player wants isn't a choice, pick the closest if it's close, otherwise leave it out, and say so in notes.
- notes: a sentence on anything the options could not express or that you approximated. Empty when everything fit.`

  if (catalog.kind === 'spell') {
    return `${common}

Spell rules:
- AOE / Targets: "single target" for one target, "N targets" for several separate targets, "split damage" for damage split among targets, a splash for a radius around a point, a line for a line.
- Duration: instantaneous unless the effect lasts. An effect that lasts and covers an area, deals dice, or has a concentration effect needs concentration; pick the matching length from either list and the app moves it to the right one.
- Challenge: "none" unless the description says how strong a target (in BP) the spell can overcome.
- Timing defaults to 2actions when the description doesn't say.
- countermagic targeting is only for a spell that counters another spell.`
  }
  return `${common}

Maneuver rules (martial techniques, non-magical):
- Range exists only for fired and thrown weapons: "within range" unless the shot reaches past twice the weapon's range. A gun's "point-blank" is for a shot within 5 ft and only counts with the split-second effect, so pick split-second with it.
- AOE / Targets: a sweep around the user is a splash, a charge or a shot through several foes is a line.
- damageDice are extra d6s on top of the weapon's own damage; don't count the weapon's damage.
- Duration says how long the effects last and multiplies their cost; instantaneous if they don't last.
- selfDebuff is a drawback the user takes on (off-balance, exposed, prone...) to lower the cost; pick one only if the description mentions a drawback. selfDuration is how long that drawback lasts.
- Timing defaults to action when the description doesn't say; "none" is for a move that takes no action at all.`
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json', 'cache-control': 'no-store' },
  })
}
