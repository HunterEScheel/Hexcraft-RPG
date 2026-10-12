import { useState } from 'react'
import { supabase } from '../lib/supabase'

/**
 * Site admin only: embeds the skill list so skill search can find skills by
 * meaning ("ride a horse" → Riding). Run it after adding skills.
 */
export function RunEmbeddings() {
  const [all, setAll] = useState(false)
  const [running, setRunning] = useState(false)
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null)

  const run = async () => {
    setRunning(true)
    setMessage(null)
    try {
      const { data } = await supabase.auth.getSession()
      const res = await fetch('/api/run-embeddings', {
        method: 'POST',
        headers: {
          'content-type': 'application/json',
          authorization: `Bearer ${data.session?.access_token ?? ''}`,
        },
        body: JSON.stringify({ all }),
      })
      const body = (await res.json().catch(() => ({}))) as {
        embedded?: number
        total?: number
        error?: string
      }
      if (!res.ok) {
        setMessage({ ok: false, text: body.error ?? `Failed (${res.status})` })
      } else if (body.total === 0) {
        setMessage({ ok: true, text: 'Every skill already has an embedding.' })
      } else {
        setMessage({ ok: true, text: `Embedded ${body.embedded} of ${body.total} skills.` })
      }
    } catch (e) {
      setMessage({ ok: false, text: e instanceof Error ? e.message : 'Failed' })
    } finally {
      setRunning(false)
    }
  }

  return (
    <section className="rounded-lg border border-zinc-800 bg-zinc-900/50 p-4 space-y-2">
      <h3 className="text-[10px] font-medium uppercase tracking-[0.3em] text-zinc-400">
        Admin · Skill embeddings
      </h3>
      <p className="text-xs text-zinc-500">
        Lets skill search find skills by meaning. Run it after adding skills to the list.
      </p>
      <div className="flex items-center gap-3 flex-wrap">
        <button
          type="button"
          onClick={run}
          disabled={running}
          className="rounded bg-violet-500 hover:bg-violet-400 disabled:opacity-40 px-3 py-1.5 text-sm font-medium text-zinc-950"
        >
          {running ? 'Running…' : 'Run embeddings'}
        </button>
        <label className="flex items-center gap-1.5 text-xs text-zinc-400">
          <input type="checkbox" checked={all} onChange={(e) => setAll(e.target.checked)} />
          Redo every skill, not just new ones
        </label>
      </div>
      {message && (
        <p className={'text-xs ' + (message.ok ? 'text-emerald-300' : 'text-rose-300')}>
          {message.text}
        </p>
      )}
    </section>
  )
}
