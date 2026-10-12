import { useEffect, useMemo, useState } from 'react'
import { skillLabel, splitSkillName, type CharacterSkill } from '../system/character'
import { COMBAT_SKILLS, isCombatSkillId } from '../system/combatSkills'
import { MAX_SKILL_LEVEL, skillCost } from '../system/costs'
import { addSkillToList, searchSkills, searchSkillsByMeaning, type SkillSearchResult } from '../lib/skills'
import { supabaseConfigured } from '../lib/supabase'
import { NumberStepper } from './NumberStepper'

interface Props {
  value: CharacterSkill[]
  onChange: (next: CharacterSkill[]) => void
}

export function SkillsEditor({ value, onChange }: Props) {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<SkillSearchResult[]>([])
  const [searching, setSearching] = useState(false)
  // Why the last add didn't happen, if it didn't.
  const [addNote, setAddNote] = useState<string | null>(null)
  // Skills that mean what was typed, found a moment later by /api/skill-search.
  const [related, setRelated] = useState<{ query: string; skills: SkillSearchResult[] }>({
    query: '',
    skills: [],
  })

  useEffect(() => {
    if (!query.trim()) {
      setResults([])
      return
    }
    let cancelled = false
    setSearching(true)
    const t = setTimeout(async () => {
      const r = await searchSkills(query)
      if (!cancelled) {
        setResults(r)
        setSearching(false)
      }
    }, 200)
    return () => {
      cancelled = true
      clearTimeout(t)
    }
  }, [query])

  // Each search by meaning costs an embedding, so wait until typing has
  // stopped for a second.
  useEffect(() => {
    if (!supabaseConfigured || query.trim().length < 3) return
    const controller = new AbortController()
    const t = setTimeout(async () => {
      const r = await searchSkillsByMeaning(query, controller.signal)
      if (!controller.signal.aborted) setRelated({ query, skills: r })
    }, 1000)
    return () => {
      controller.abort()
      clearTimeout(t)
    }
  }, [query])
  // The top matches by meaning for the current text, even ones that also
  // matched by name.
  const relatedOnly =
    related.query === query && query.trim().length >= 3 ? related.skills : []

  const { saveSkills, combatSkills, customSkills } = useMemo(() => {
    const combatById = new Map(value.filter((s) => isCombatSkillId(s.id)).map((s) => [s.id, s]))
    const ordered = COMBAT_SKILLS.map((def) => ({
      def,
      skill: combatById.get(def.id) ?? { id: def.id, name: def.name, level: 0 },
    }))
    const saves = ordered.filter((o) => o.def.category === 'passive')
    const others = ordered.filter((o) => o.def.category !== 'passive')
    const custom = value.filter((s) => !isCombatSkillId(s.id))
    return {
      saveSkills: saves,
      combatSkills: others,
      customSkills: custom,
    }
  }, [value])

  const setLevel = (id: string, level: number) =>
    onChange(value.map((s) => (s.id === id ? { ...s, level } : s)))

  const remove = (id: string) => {
    if (isCombatSkillId(id)) return
    onChange(value.filter((s) => s.id !== id))
  }

  // A specificity is the character's own: one character takes Driving for
  // motorcycles, another for airships. Typed text like "Driving (Motorcycles)"
  // becomes a name and a specificity, and the same skill can be taken more than
  // once with different specificities. A typed skill that isn't on the list
  // is added to it for everyone.
  const addSkill = (name: string, specificity?: string, custom = false) => {
    const typed = { name: name.trim(), specificity: specificity?.trim() || undefined }
    if (!typed.name) return
    const label = skillLabel(typed).toLowerCase()
    if (COMBAT_SKILLS.some((c) => c.name.toLowerCase() === typed.name.toLowerCase())) {
      setAddNote(`${typed.name} is a combat skill; raise it above instead.`)
      return
    }
    // A skill can be taken again only if every copy has its own specificity.
    const same = value.find((s) => skillLabel(s).toLowerCase() === label)
    if (same) {
      setAddNote(
        same.specificity?.trim()
          ? `You already have ${skillLabel(same)}.`
          : `You already have ${same.name}. Give it a specificity first, then you can add ${same.name} again with another.`,
      )
      return
    }
    setAddNote(null)
    if (custom) void addSkillToList(typed.name)
    onChange([
      ...value,
      {
        id: `custom-${crypto.randomUUID()}`,
        name: typed.name,
        level: 1,
        ...(typed.specificity ? { specificity: typed.specificity } : {}),
      },
    ])
    setQuery('')
    setResults([])
  }
  const typedSpecificity = splitSkillName(query).specificity

  const setSpecificity = (id: string, specificity: string) =>
    onChange(value.map((s) => (s.id === id ? { ...s, specificity } : s)))

  const renderResult = (r: SkillSearchResult, group: string) => (
    <button
      key={`${group}-${r.id}`}
      type="button"
      onClick={() => addSkill(r.name, typedSpecificity)}
      className="w-full px-3 py-2 text-left hover:bg-zinc-900"
    >
      <div className="flex items-baseline justify-between gap-2">
        <span className="text-sm text-zinc-100">
          {r.name}
          {typedSpecificity && <span className="text-zinc-400"> ({typedSpecificity})</span>}
        </span>
        {r.similarity !== undefined && (
          <span className="text-[10px] font-mono text-zinc-500">
            {Math.round(r.similarity * 100)}% match
          </span>
        )}
      </div>
      {r.description && (
        <div className="text-xs text-zinc-500 line-clamp-1">{r.description}</div>
      )}
    </button>
  )

  const renderSkillRow = ({
    def,
    skill,
  }: {
    def: (typeof COMBAT_SKILLS)[number]
    skill: CharacterSkill
  }) => (
    <li
      key={skill.id}
      className="flex items-center justify-between gap-3 rounded border border-zinc-800 bg-zinc-900 px-3 py-2"
    >
      <div className="flex-1 min-w-0">
        <div className="text-sm text-zinc-100 truncate">{skill.name}</div>
        <div className="text-xs text-zinc-500 truncate">{def.effect}</div>
      </div>
      <span className="text-xs text-zinc-500 font-mono w-16 text-right">
        {skillCost(skill.level)} BP
      </span>
      <NumberStepper
        value={skill.level}
        onChange={(v) => setLevel(skill.id, v)}
        min={0}
        max={MAX_SKILL_LEVEL}
      />
    </li>
  )

  return (
    <div className="space-y-6">
      <div>
        <h4 className="text-xs uppercase tracking-wide text-zinc-400 mb-2">
          Saving throws
        </h4>
        <p className="text-xs text-zinc-500 mb-2">
          Dodge, Grit, and Resolve are your three saves. Each level adds +1
          to the corresponding save roll.
        </p>
        <ul className="space-y-2">{saveSkills.map(renderSkillRow)}</ul>
      </div>

      <div>
        <h4 className="text-xs uppercase tracking-wide text-zinc-400 mb-2">
          Combat skills
        </h4>
        <p className="text-xs text-zinc-500 mb-2">
          Every character starts with these at level 0. Upgrade as needed; they
          can&apos;t be removed.
        </p>
        <ul className="space-y-2">{combatSkills.map(renderSkillRow)}</ul>
      </div>

      <div>
        <h4 className="text-xs uppercase tracking-wide text-zinc-400 mb-2">
          Other skills
        </h4>
        <div>
          <input
            type="text"
            value={query}
            onChange={(e) => {
              setQuery(e.target.value)
              setAddNote(null)
            }}
            placeholder={
              supabaseConfigured
                ? 'Search non-combat skills, or type your own…'
                : 'Add a skill (Supabase not configured — custom only)'
            }
            className="w-full bg-zinc-900 border border-zinc-700 rounded px-3 py-2 text-zinc-100 placeholder:text-zinc-500"
          />
          {query && (
            <div className="mt-2 rounded border border-zinc-800 bg-zinc-950 max-h-56 overflow-y-auto divide-y divide-zinc-800">
              {searching && (
                <div className="px-3 py-2 text-xs text-zinc-500">Searching…</div>
              )}
              {results.map((r) => renderResult(r, 'name'))}
              {relatedOnly.length > 0 && (
                <div className="px-3 pt-2 pb-1 text-[10px] uppercase tracking-wider text-zinc-500">
                  Related skills
                </div>
              )}
              {relatedOnly.map((r) => renderResult(r, 'meaning'))}
              {/* Skill names are freeform: whatever is typed can be added as-is,
                  e.g. a listed skill with a tech level or specialty tacked on. */}
              {!searching &&
                !results.some((r) => r.name.toLowerCase() === splitSkillName(query).name.toLowerCase()) && (
                  <button
                    type="button"
                    onClick={() => {
                      const typed = splitSkillName(query)
                      addSkill(typed.name, typed.specificity, true)
                    }}
                    className="w-full px-3 py-2 text-left text-sm text-amber-300 hover:bg-zinc-900"
                  >
                    + Add &quot;{query.trim()}&quot;{results.length + relatedOnly.length === 0 ? ' as custom skill' : ' as typed'}
                  </button>
                )}
            </div>
          )}
        </div>

        {addNote && <p className="mt-2 text-xs text-amber-300">{addNote}</p>}

        {customSkills.length === 0 ? (
          <p className="text-sm text-zinc-500 italic mt-3">No other skills yet.</p>
        ) : (
          <ul className="space-y-2 mt-3">
            {customSkills.map((s) => (
              <li
                key={s.id}
                className="flex items-center justify-between gap-3 rounded border border-zinc-800 bg-zinc-900 px-3 py-2"
              >
                <span className="text-sm text-zinc-100 truncate">{s.name}</span>
                <input
                  type="text"
                  value={s.specificity ?? ''}
                  onChange={(e) => setSpecificity(s.id, e.target.value)}
                  placeholder="Specificity, e.g. Motorcycles (optional)"
                  aria-label={`${s.name} specificity`}
                  className="flex-1 min-w-0 bg-zinc-950 border border-zinc-800 rounded px-2 py-1 text-xs text-zinc-200 placeholder:text-zinc-600"
                />
                <span className="text-xs text-zinc-500 font-mono w-16 text-right">
                  {skillCost(s.level)} BP
                </span>
                <NumberStepper
                  value={s.level}
                  onChange={(v) => setLevel(s.id, v)}
                  min={1}
                  max={MAX_SKILL_LEVEL}
                />
                <button
                  type="button"
                  onClick={() => remove(s.id)}
                  className="text-zinc-500 hover:text-rose-400 text-sm"
                  aria-label={`Remove ${s.name}`}
                >
                  ✕
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
