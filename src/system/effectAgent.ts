// The "describe it" agent: a plain-language description of a spell or
// maneuver goes to /api/effect-agent, which asks a model to pick from the
// builder's own options. This module is the client half: it lists those
// options as a catalog the model chooses from, and turns the model's answer
// back into a SpellDraft, checking every pick against the criteria.
//
// The model names its picks by text ("Splash: 20 ft splash"), not by index, so
// a stale or invented answer is caught here instead of silently picking the
// wrong option.
import {
  CASTING_TIMES,
  REPEATABLE_CRITERIA,
  SPELL_TARGETING_OPTIONS,
  applyConcentrationRule,
  costMultiplierApplies,
  emptySpellDraft,
  selectedOption,
  type CastingTimeKey,
  type CriterionKey,
  type SpellCriterion,
  type SpellDraft,
  type SpellSelection,
  type SpellTargeting,
} from './spells'

export type EffectKind = 'spell' | 'maneuver'

/** One criterion as the model sees it: the choices it may name. */
export interface AgentCriterion {
  key: CriterionKey
  label: string
  /** Picked as a list (buffs/debuffs, self-debuffs) rather than once. */
  repeatable: boolean
  choices: string[]
}

/** Everything the model may pick from, sent with the description. */
export interface AgentCatalog {
  kind: EffectKind
  criteria: AgentCriterion[]
  castingTimes: { key: CastingTimeKey; label: string }[]
  targetings: { key: SpellTargeting; label: string }[]
}

/** The model's answer, as the response schema built from a catalog shapes it. */
export interface AgentOutput {
  name: string
  /** Each once-only criterion's choice. */
  picks: Partial<Record<CriterionKey, string>>
  /** Each repeatable criterion's choices. */
  lists: Partial<Record<CriterionKey, string[]>>
  damageDice: number
  castingTime: string
  targeting: string
  /** What the description asked for that the options can't express. */
  notes: string
}

export interface AgentDraft {
  draft: SpellDraft
  name: string
  notes: string
  /** Picks that were invalid or adjusted, for the user to check. */
  warnings: string[]
}

/** The text the model names an option by: "Mode: option" when there are modes. */
function choiceText(modeLabel: string | null, optionLabel: string): string {
  return modeLabel ? `${modeLabel}: ${optionLabel}` : optionLabel
}

/**
 * Every choice a criterion offers, with the selection it stands for. A
 * repeatable criterion leaves out its "none" picks: an empty list says that.
 */
export function criterionChoices(criterion: SpellCriterion): Map<string, SpellSelection> {
  const repeatable = REPEATABLE_CRITERIA.includes(criterion.key)
  const choices = new Map<string, SpellSelection>()
  const add = (modeLabel: string | null, modeIndex: number, options: SpellCriterion['options']) =>
    (options ?? []).forEach((o, optionIndex) => {
      if (repeatable && o.tier === 0) return
      choices.set(choiceText(modeLabel, o.label), { modeIndex, optionIndex })
    })
  if (criterion.options) add(null, 0, criterion.options)
  criterion.modes?.forEach((m, modeIndex) =>
    add((criterion.modes?.length ?? 0) > 1 ? m.label : null, modeIndex, m.options),
  )
  return choices
}

const castingTimesFor = (kind: EffectKind) =>
  CASTING_TIMES.filter((t) => kind === 'maneuver' || !t.maneuverOnly)

const targetingsFor = (kind: EffectKind) =>
  SPELL_TARGETING_OPTIONS.filter((t) => kind === 'spell' || !t.spellOnly)

/** The catalog for these criteria: what the builder would offer right now. */
export function agentCatalog(criteria: readonly SpellCriterion[], kind: EffectKind): AgentCatalog {
  return {
    kind,
    criteria: criteria.map((c) => ({
      key: c.key,
      label: c.label,
      repeatable: REPEATABLE_CRITERIA.includes(c.key),
      choices: [...criterionChoices(c).keys()],
    })),
    castingTimes: castingTimesFor(kind).map((t) => ({ key: t.key, label: t.label })),
    targetings: targetingsFor(kind).map((t) => ({ key: t.key, label: t.label })),
  }
}

const isRecord = (v: unknown): v is Record<string, unknown> =>
  typeof v === 'object' && v !== null && !Array.isArray(v)

const MAX_DAMAGE_DICE = 100

/**
 * Turns the model's answer into a draft for these criteria. Anything that
 * doesn't match an option is left at the builder's default and reported in
 * `warnings`; nothing invalid reaches the draft.
 */
export function draftFromAgent(
  raw: unknown,
  criteria: readonly SpellCriterion[],
  kind: EffectKind,
): AgentDraft {
  const warnings: string[] = []
  const out = isRecord(raw) ? raw : {}
  if (!isRecord(raw)) warnings.push('The answer was not an object; nothing was filled in.')
  const picks = isRecord(out.picks) ? out.picks : {}
  const lists = isRecord(out.lists) ? out.lists : {}

  const empty = emptySpellDraft()
  const selections = { ...empty.selections }
  const extras: Partial<Record<CriterionKey, SpellSelection[]>> = {}

  for (const c of criteria) {
    const choices = criterionChoices(c)
    if (REPEATABLE_CRITERIA.includes(c.key)) {
      const named = Array.isArray(lists[c.key]) ? (lists[c.key] as unknown[]) : []
      const seen = new Set<string>()
      const valid: SpellSelection[] = []
      for (const item of named) {
        const sel = typeof item === 'string' ? choices.get(item) : undefined
        if (!sel) {
          warnings.push(`${c.label}: "${String(item)}" is not an option; left out.`)
        } else if (!seen.has(item as string)) {
          seen.add(item as string)
          valid.push(sel)
        }
      }
      if (valid.length > 0) {
        selections[c.key] = valid[0]
        extras[c.key] = valid.slice(1)
      }
      continue
    }
    const name = picks[c.key]
    const sel = typeof name === 'string' ? choices.get(name) : undefined
    if (sel) selections[c.key] = sel
    else if (name !== undefined) warnings.push(`${c.label}: "${String(name)}" is not an option; left at the default.`)
  }

  let damageDice = 0
  if (typeof out.damageDice === 'number' && Number.isFinite(out.damageDice)) {
    damageDice = Math.min(MAX_DAMAGE_DICE, Math.max(0, Math.round(out.damageDice)))
    if (damageDice !== out.damageDice) warnings.push(`Damage dice ${out.damageDice} became ${damageDice}.`)
  } else if (out.damageDice !== undefined) {
    warnings.push('Damage dice were not a number; set to 0.')
  }

  const castingTime = castingTimesFor(kind).find((t) => t.key === out.castingTime)?.key
  if (!castingTime && out.castingTime !== undefined) {
    warnings.push(`Timing "${String(out.castingTime)}" is not allowed; left at the default.`)
  }
  const targeting = targetingsFor(kind).find((t) => t.key === out.targeting)?.key
  if (!targeting && out.targeting !== undefined) {
    warnings.push(`Targeting "${String(out.targeting)}" is not allowed; left at the default.`)
  }

  let draft: SpellDraft = {
    ...empty,
    selections,
    damageDice,
    castingTime: castingTime ?? empty.castingTime,
    targeting: targeting ?? empty.targeting,
  }
  if (extras.buffDebuff?.length) draft.extraEffects = extras.buffDebuff
  if (extras.selfDebuff?.length) draft.extraSelfDebuffs = extras.selfDebuff

  // A cost multiplier that needs an effect (a gun's point-blank needs
  // split-second) is kept but flagged when that effect is missing.
  for (const c of criteria) {
    const opt = selectedOption(c, draft.selections[c.key])
    if (opt?.requiresEffect && !costMultiplierApplies(opt, draft, criteria)) {
      warnings.push(`${c.label} "${opt.label}" needs the ${opt.requiresEffect} effect to count.`)
    }
  }

  // The same rule the builder applies on every change.
  const before = draft.selections.duration
  draft = applyConcentrationRule(draft, criteria)
  if (draft.selections.duration !== before) {
    warnings.push('Duration moved to the list the concentration rule allows.')
  }

  return {
    draft,
    name: typeof out.name === 'string' ? out.name.trim() : '',
    notes: typeof out.notes === 'string' ? out.notes.trim() : '',
    warnings,
  }
}
