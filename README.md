# Hexcraft RPG

Character builder for the Hexcraft RPG system, served at the root of
[rpg.jaeg.click](https://rpg.jaeg.click). Build characters against a power-tier
budget, keep live sheets for play, and stat up monsters.

## Stack

- Vite + React 19 + TypeScript
- Tailwind CSS v4 (`@tailwindcss/vite`)
- Supabase (Postgres + pgvector) for sign-in, saved characters and skill search
- oxlint, Vitest

## Environment

Copy `.env.example` to `.env.local` and fill in the Supabase project URL and anon
key (Project Settings → API):

```
VITE_SUPABASE_URL=https://YOUR-PROJECT.supabase.co
VITE_SUPABASE_ANON_KEY=YOUR-ANON-KEY
```

The client throws at load without them. The same two variables go in Vercel under
**Project Settings → Environment Variables**.

No account is needed: anyone can browse, create and edit every character. Signing
in (GitHub, Discord or an email link, at `/sign-in`) only unlocks deleting them.
Sign-in returns to `https://rpg.jaeg.click/`, which must be in the Supabase
project's allowed redirect URLs. The policies behind this are
`supabase/migrations/hexcraft_0002_public_characters.sql` in the jaeg.click repo.

## Commands

```bash
npm install
npm run dev         # local dev server
npm run build       # typecheck + production build to dist/
npm run lint        # oxlint
npm test            # vitest (no tests yet)
npm run preview     # serve dist/
npm run embeddings  # populate skill-search embeddings (see below)
```

## Skill embeddings

Skill search reads `hexcraft_skill_embeddings`. The skill list is deliberately not
in the repository: put yours in `scripts/hexcraft-skills.txt` (gitignored), one
`Name | optional description` per line, then set the server-only variables and run
`npm run embeddings`:

```sh
export OPENAI_API_KEY=...
export SUPABASE_URL=https://YOUR-PROJECT.supabase.co
export SUPABASE_SERVICE_ROLE_KEY=...   # bypasses RLS — never commit, never VITE_-prefix
npm run embeddings
```

It batches 64 skills per OpenAI call and upserts on the skill name. Without
embeddings, skill search returns nothing and custom skills still work. More detail
in [docs/hexcraft.md](docs/hexcraft.md).

## Supabase schema

The `hexcraft_*` tables and RPC live in the shared Supabase project, and their
migrations (`hexcraft_*`) live in the
[HunterEScheel/jaeg.click](https://github.com/HunterEScheel/jaeg.click) repository
under `supabase/`. Don't create or run migrations from here.

## Deploying

Vercel, using `vercel.json`: Vite framework preset, `npm run build` → `dist/`, and
every path rewritten to `index.html` so deep links like `/sheet/<id>` load the app.

## System reference

| Cost                      | Rate                                                  |
| ------------------------- | ----------------------------------------------------- |
| HP                        | 2 BP per 3 HP                                         |
| EP                        | 2 BP per 1 EP                                         |
| Skill level               | exponential ladder, see `src/system/costs.ts`         |
| Attribute / magic school  | the skill ladder from an offset of 4                  |
| Magic medium              | the skill ladder from an offset of 3                  |

Power tier budgets range from 150 BP (Peasants) to 2000 BP (World Savior).
