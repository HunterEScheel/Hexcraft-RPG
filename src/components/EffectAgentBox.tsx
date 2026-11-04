import { useState } from 'react'
import { supabase } from '../lib/supabase'
import { useIsAdmin, useUser } from '../lib/useUser'
import { agentCatalog, draftFromAgent, type AgentDraft, type EffectKind } from '../system/effectAgent'
import type { SpellCriterion, SpellDraft } from '../system/spells'

/**
 * Site admin only: describe a spell or maneuver in plain words and have the
 * agent (/api/effect-agent) fill in the builder below. The picks land in the
 * builder for review; nothing is spent or saved.
 */
export function EffectAgentBox({
  kind,
  criteria,
  onDraft,
}: {
  kind: EffectKind
  /** The criteria the builder is showing now. */
  criteria: readonly SpellCriterion[]
  /** Load the built draft (and its suggested name) into the builder. */
  onDraft: (draft: SpellDraft, name: string) => void
}) {
  const isAdmin = useIsAdmin(useUser())
  const [description, setDescription] = useState('')
  const [running, setRunning] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [result, setResult] = useState<AgentDraft | null>(null)

  if (!isAdmin) return null

  const run = async () => {
    setRunning(true)
    setError(null)
    setResult(null)
    try {
      const { data } = await supabase.auth.getSession()
      const res = await fetch('/api/effect-agent', {
        method: 'POST',
        headers: {
          'content-type': 'application/json',
          authorization: `Bearer ${data.session?.access_token ?? ''}`,
        },
        body: JSON.stringify({ description, catalog: agentCatalog(criteria, kind) }),
      })
      const body = (await res.json().catch(() => ({}))) as { output?: unknown; error?: string }
      if (!res.ok || body.output === undefined) {
        setError(body.error ?? `Failed (${res.status})`)
        return
      }
      const built = draftFromAgent(body.output, criteria, kind)
      onDraft(built.draft, built.name)
      setResult(built)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed')
    } finally {
      setRunning(false)
    }
  }

  const tooShort = description.trim().length < 3

  return (
    <div className="rounded border border-violet-800/50 bg-violet-950/20 p-3 space-y-2">
      <span className="block text-xs uppercase tracking-wide text-violet-300/80">
        Describe it · admin
      </span>
      <textarea
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        rows={2}
        maxLength={1000}
        placeholder={
          kind === 'spell'
            ? 'A fireball that hits everyone within 20 ft, 6d6 fire damage, takes an action'
            : 'A sweeping axe strike that hits everyone around me and knocks them prone'
        }
        className="w-full bg-zinc-900 border border-zinc-700 rounded px-2 py-1.5 text-sm text-zinc-100"
      />
      <div className="flex items-center gap-3 flex-wrap">
        <button
          type="button"
          onClick={run}
          disabled={running || tooShort}
          className="rounded bg-violet-500 hover:bg-violet-400 disabled:opacity-40 disabled:cursor-not-allowed px-3 py-1.5 text-sm font-medium text-zinc-950"
        >
          {running ? 'Building…' : `Build ${kind}`}
        </button>
        <span className="text-[11px] text-zinc-500">Fills in the builder below for you to check.</span>
      </div>
      {error && <p className="text-xs text-rose-300">{error}</p>}
      {result && (
        <div className="space-y-1 text-xs">
          <p className="text-emerald-300">
            Built{result.name ? ` "${result.name}"` : ''}. Check the picks below.
          </p>
          {result.notes && <p className="text-zinc-400">{result.notes}</p>}
          {result.warnings.map((w) => (
            <p key={w} className="text-amber-300">
              {w}
            </p>
          ))}
        </div>
      )}
    </div>
  )
}
