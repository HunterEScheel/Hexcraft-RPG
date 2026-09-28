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

No account is needed. A character can be locked with an optional passcode when
it is made: then only someone who enters that passcode (remembered in their
browser afterwards) can view, edit or delete it. Open characters are anyone's to
view and edit, and any signed-in account can delete them. The site admin
(`site_admins`) can open, edit and delete every character and sees each
passcode on the roster. Signing in (GitHub, Discord or an email link, at
`/sign-in`) is only needed to delete open characters or to act as the admin.
Sign-in returns to `https://rpg.jaeg.click/`, which must be in the Supabase
project's allowed redirect URLs. The rules live in the database functions of
`supabase/migrations/hexcraft_0003_character_passcodes.sql` in the jaeg.click repo.

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

## Player's Guide

The rules are published as `public/hexcraft-players-guide.pdf`, which the
Running the game page offers as a download. It is generated, not written by
hand: edit `scripts/players-guide/build.py` (its numbers are copied from
`src/system`, so update them there when a rule changes), then rebuild it:

```bash
npm i --no-save playwright   # if Playwright isn't installed
python3 scripts/players-guide/build.py && node scripts/players-guide/pdf.mjs
```

The blank, printable character sheet (`public/hexcraft-character-sheet.pdf`) is
built the same way from `scripts/character-sheet/`:

```bash
python3 scripts/character-sheet/build.py && node scripts/character-sheet/pdf.mjs
```
