#!/usr/bin/env node
// Generate OpenAI embeddings for the non-combat skill list in Supabase.
//
// Usage:
//   1. Put the keys in .env.local (git-ignored) or set them in the shell:
//        OPENAI_API_KEY=sk-...
//        SUPABASE_SERVICE_ROLE_KEY=...   (Supabase → Project Settings → API)
//        SUPABASE_URL=https://xyz.supabase.co   (VITE_SUPABASE_URL works too)
//   2. npm run embeddings
//
// By default it embeds every skill already in hexcraft_skill_embeddings that
// has no embedding yet (e.g. ones added by SQL). If scripts/hexcraft-skills.txt
// exists (one "Name | description" per line, description optional), those
// skills are upserted and embedded instead.

import { existsSync, readFileSync } from 'node:fs'
import { createClient } from '@supabase/supabase-js'

for (const file of ['.env', '.env.local']) {
  if (existsSync(file)) process.loadEnvFile(file)
}

const OPENAI_API_KEY = process.env.OPENAI_API_KEY
const SUPABASE_URL = process.env.SUPABASE_URL || process.env.VITE_SUPABASE_URL
const SUPABASE_SERVICE_ROLE_KEY = process.env.SUPABASE_SERVICE_ROLE_KEY
const MODEL = process.env.EMBEDDING_MODEL || 'text-embedding-3-small'
const BATCH_SIZE = 64
const SKILL_FILE = 'scripts/hexcraft-skills.txt'

const missing = Object.entries({ OPENAI_API_KEY, SUPABASE_URL, SUPABASE_SERVICE_ROLE_KEY })
  .filter(([, v]) => !v)
  .map(([k]) => k)
if (missing.length > 0) {
  console.error(`Missing: ${missing.join(', ')}. Put them in .env.local or set them in the shell.`)
  process.exit(1)
}

const supabase = createClient(SUPABASE_URL, SUPABASE_SERVICE_ROLE_KEY)

async function embedBatch(inputs) {
  const res = await fetch('https://api.openai.com/v1/embeddings', {
    method: 'POST',
    headers: {
      'content-type': 'application/json',
      authorization: `Bearer ${OPENAI_API_KEY}`,
    },
    body: JSON.stringify({ model: MODEL, input: inputs }),
  })
  if (!res.ok) {
    throw new Error(`OpenAI ${res.status}: ${await res.text()}`)
  }
  const json = await res.json()
  return json.data.map((d) => d.embedding)
}

const text = (s) => (s.description ? `${s.name}: ${s.description}` : s.name)

if (existsSync(SKILL_FILE)) {
  const skills = readFileSync(SKILL_FILE, 'utf8')
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
    .map((line) => {
      const [name, ...descParts] = line.split('|').map((s) => s.trim())
      return { name, description: descParts.join(' | ') || null }
    })
  console.log(`Embedding ${skills.length} skills from ${SKILL_FILE} with ${MODEL}...`)
  for (let i = 0; i < skills.length; i += BATCH_SIZE) {
    const batch = skills.slice(i, i + BATCH_SIZE)
    const embeddings = await embedBatch(batch.map(text))
    const { error } = await supabase.from('hexcraft_skill_embeddings').upsert(
      batch.map((s, j) => ({ skill_name: s.name, description: s.description, embedding: embeddings[j] })),
      { onConflict: 'skill_name' },
    )
    if (error) {
      console.error('upsert failed', error)
      process.exit(1)
    }
    console.log(`  ${Math.min(i + BATCH_SIZE, skills.length)}/${skills.length}`)
  }
} else {
  const { data, error } = await supabase
    .from('hexcraft_skill_embeddings')
    .select('id, skill_name, description')
    .is('embedding', null)
    .order('skill_name')
  if (error) {
    console.error('reading skills failed', error)
    process.exit(1)
  }
  const skills = data.map((r) => ({ id: r.id, name: r.skill_name, description: r.description }))
  console.log(`Embedding ${skills.length} skills without an embedding, with ${MODEL}...`)
  for (let i = 0; i < skills.length; i += BATCH_SIZE) {
    const batch = skills.slice(i, i + BATCH_SIZE)
    const embeddings = await embedBatch(batch.map(text))
    for (const [j, s] of batch.entries()) {
      const { error: updateError } = await supabase
        .from('hexcraft_skill_embeddings')
        .update({ embedding: embeddings[j] })
        .eq('id', s.id)
      if (updateError) {
        console.error(`update failed for ${s.name}`, updateError)
        process.exit(1)
      }
    }
    console.log(`  ${Math.min(i + BATCH_SIZE, skills.length)}/${skills.length}`)
  }
}

console.log('done.')
