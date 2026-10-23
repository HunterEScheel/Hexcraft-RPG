// Spell scaling criteria from the Hexcraft RPG sheet.
// Each criterion is split into one or more "modes" (e.g. AOE: splash/line/targets).
// Each mode contains options whose tier drives the EP cost. The per-tier rates
// are half the sheet's; damage dice keep their full EP_PER_DAMAGE_DIE.

export type CriterionKey =
  | 'range'
  | 'aoe'
  | 'duration'
  | 'buffDebuff'
  | 'challenge'
  // Maneuvers only: a drawback the user takes on, and for how long.
  | 'selfDebuff'
  | 'selfDuration'

export interface SpellOption {
  label: string
  tier: number
  /** For a `multiplies` criterion: what it multiplies the other one's EP by. */
  multiplier?: number
}

export interface SpellMode {
  key: string
  label: string
  options: readonly SpellOption[]
}

// Either `options` (simple list) or `modes` (mode-then-value pick).
export interface SpellCriterion {
  key: CriterionKey
  label: string
  epPerTier: number
  options?: readonly SpellOption[]
  modes?: readonly SpellMode[]
  /** Takes EP off the cost instead of adding it (a maneuver's self-debuff). */
  refund?: boolean
  /** Counts only when this other criterion is above tier 0. */
  onlyWith?: CriterionKey
  /**
   * Instead of adding EP, multiplies this other criterion's total (every pick
   * of it) by the chosen option's multiplier: a maneuver's durations.
   */
  multiplies?: CriterionKey
}

export const SPELL_CRITERIA: readonly SpellCriterion[] = [
  {
    key: 'range',
    label: 'Range',
    epPerTier: 4,
    options: [
      { label: 'touch / 30 ft', tier: 0 },
      { label: '120 ft', tier: 1 },
      { label: '600 ft', tier: 2 },
      { label: '1 mile', tier: 3 },
      { label: 'any distance', tier: 4 },
      { label: 'cross-planar', tier: 5 },
    ],
  },
  {
    key: 'aoe',
    label: 'AOE / Targets',
    epPerTier: 5,
    modes: [
      {
        key: 'targets',
        label: 'Targeted',
        options: [
          { label: 'none / self', tier: 0 },
          { label: 'single target', tier: 0 },
          { label: 'split damage', tier: 1 },
          { label: '3 targets', tier: 2 },
          { label: '10 targets', tier: 3 },
          { label: '100 targets', tier: 5 },
        ],
      },
      {
        key: 'splash',
        label: 'Splash',
        options: [
          { label: '20 ft splash', tier: 1 },
          { label: '60 ft splash', tier: 2 },
          { label: '120 ft splash', tier: 3 },
          { label: '300 ft splash', tier: 4 },
        ],
      },
      {
        key: 'line',
        label: 'Line',
        options: [
          { label: '60 ft line', tier: 1 },
          { label: '300 ft line', tier: 2 },
          { label: '600 ft line', tier: 3 },
          { label: '1 mile line', tier: 4 },
        ],
      },
    ],
  },
  {
    key: 'duration',
    label: 'Duration',
    epPerTier: 4,
    modes: [
      {
        key: 'standard',
        label: 'No concentration',
        options: [
          { label: 'instantaneous', tier: 0 },
          { label: '1 minute', tier: 1 },
          { label: '1 hour', tier: 2 },
          { label: '1 day', tier: 3 },
          { label: '1 year', tier: 4 },
          { label: 'permanent', tier: 5 },
        ],
      },
      {
        key: 'concentration',
        label: 'Concentration',
        options: [
          { label: '1 round', tier: 1 },
          { label: '1 minute', tier: 2 },
          { label: '1 hour', tier: 3 },
          { label: '8 hours', tier: 4 },
        ],
      },
    ],
  },
  {
    key: 'buffDebuff',
    label: 'Buff / Debuff',
    epPerTier: 5,
    modes: [
      {
        key: 'none',
        label: 'None',
        options: [{ label: 'none', tier: 0 }],
      },
      {
        key: 't1',
        label: 'Tier I (5 EP)',
        options: [
          { label: 'darkvision', tier: 1 },
          { label: 'magic detection', tier: 1 },
          { label: 'languages', tier: 1 },
          { label: '1-sense illusions', tier: 1 },
          { label: '+2 skill', tier: 1 },
          { label: 'cursory knowledge', tier: 1 },
          { label: 'inconvenience', tier: 1 },
          { label: 'poisoned', tier: 1 },
          { label: 'deafened', tier: 1 },
          { label: 'grappled', tier: 1 },
          { label: 'slowed', tier: 1 },
          { label: 'reincarnation', tier: 1 },
        ],
      },
      {
        key: 't2',
        label: 'Tier II (10 EP)',
        options: [
          { label: 'tremorsense', tier: 2 },
          { label: 'see invisibility', tier: 2 },
          { label: 'detect lies', tier: 2 },
          { label: 'multi-sense illusions', tier: 2 },
          { label: '+5 skill', tier: 2 },
          { label: 'fundamental knowledge', tier: 2 },
          { label: 'prone', tier: 2 },
          { label: 'frightened', tier: 2 },
          { label: 'illness', tier: 2 },
          { label: 'undeath', tier: 2 },
          { label: 'exhaustion', tier: 2 },
        ],
      },
      {
        key: 't3',
        label: 'Tier III (15 EP)',
        options: [
          { label: 'truesight', tier: 3 },
          { label: 'total evasion', tier: 3 },
          { label: 'detailed knowledge', tier: 3 },
          { label: 'disability', tier: 3 },
          { label: 'restrained', tier: 3 },
          { label: 'blinded', tier: 3 },
          { label: 'silenced', tier: 3 },
          { label: 'charmed', tier: 3 },
          { label: 'resurrection', tier: 3 },
        ],
      },
      {
        key: 't4',
        label: 'Tier IV (20 EP)',
        options: [
          { label: 'debilitated', tier: 4 },
          { label: 'banished', tier: 4 },
          { label: 'dominated', tier: 4 },
          { label: 'polymorphed', tier: 4 },
          { label: 'total knowledge', tier: 4 },
          { label: 'true resurrection', tier: 4 },
        ],
      },
      {
        key: 't5',
        label: 'Tier V (25 EP)',
        options: [
          { label: 'soul-bound', tier: 5 },
          { label: 'death', tier: 5 },
          { label: 'True Resurrection (greater)', tier: 5 },
        ],
      },
    ],
  },
  {
    key: 'challenge',
    label: 'Challenge',
    epPerTier: 8,
    options: [
      { label: 'none', tier: 0 },
      { label: '150 BP', tier: 0 },
      { label: '400 BP', tier: 1 },
      { label: '1 000 BP', tier: 2 },
      { label: '2 500 BP', tier: 3 },
      { label: '5 000 BP', tier: 4 },
      { label: '10 000 BP', tier: 5 },
    ],
  },
] as const

export type CastingTimeKey =
  | 'none'
  | 'reaction'
  | 'action'
  | '2actions'
  | '1round'
  | '1minute'
  | '1hour'

export interface CastingTimeOption {
  key: CastingTimeKey
  label: string
  multiplier: number
  /** Only maneuvers can use it. */
  maneuverOnly?: boolean
}

export const CASTING_TIMES: readonly CastingTimeOption[] = [
  // A maneuver that takes no action at all, at eight times the price.
  { key: 'none', label: 'None (no action)', multiplier: 8, maneuverOnly: true },
  { key: 'reaction', label: 'Reaction', multiplier: 4 },
  { key: 'action', label: 'Action', multiplier: 2 },
  { key: '2actions', label: '2 Actions', multiplier: 1 },
  { key: '1round', label: '1 Round', multiplier: 0.75 },
  { key: '1minute', label: '1 Minute', multiplier: 0.5 },
  { key: '1hour', label: '1 Hour', multiplier: 0.25 },
] as const

/** The timings a spell can use: all but the maneuver-only ones. */
export const SPELL_CASTING_TIMES = CASTING_TIMES.filter((t) => !t.maneuverOnly)

export const EP_PER_DAMAGE_DIE = 2

export interface SpellSelection {
  modeIndex: number
  optionIndex: number
}

export type SpellTargeting = 'hit' | 'dodge' | 'grit' | 'resolve' | 'countermagic'

export const SPELL_TARGETING_OPTIONS: readonly {
  key: SpellTargeting
  label: string
}[] = [
  { key: 'hit', label: 'Roll to hit' },
  { key: 'dodge', label: 'Dodge save' },
  { key: 'grit', label: 'Grit save' },
  { key: 'resolve', label: 'Resolve save' },
  { key: 'countermagic', label: 'Countermagic check' },
] as const

export interface SpellDraft {
  selections: Record<CriterionKey, SpellSelection>
  /**
   * Buffs/debuffs beyond the first (selections.buffDebuff), each priced on
   * its own. Drafts saved before there could be several have none.
   */
  extraEffects?: SpellSelection[]
  /** A maneuver's self-debuffs beyond the first, each refunded on its own. */
  extraSelfDebuffs?: SpellSelection[]
  damageDice: number
  castingTime: CastingTimeKey
  targeting: SpellTargeting
}

export function targetingName(t: SpellTargeting): string {
  return t === 'dodge'
    ? 'Dodge'
    : t === 'grit'
      ? 'Grit'
      : t === 'resolve'
        ? 'Resolve'
        : t === 'countermagic'
          ? 'Countermagic'
          : 'Hit'
}

export function emptySpellDraft(): SpellDraft {
  return {
    selections: {
      range: { modeIndex: 0, optionIndex: 0 },
      aoe: { modeIndex: 0, optionIndex: 0 },
      duration: { modeIndex: 0, optionIndex: 0 },
      buffDebuff: { modeIndex: 0, optionIndex: 0 },
      challenge: { modeIndex: 0, optionIndex: 0 },
      selfDebuff: { modeIndex: 0, optionIndex: 0 },
      selfDuration: { modeIndex: 0, optionIndex: 0 },
    },
    damageDice: 0,
    castingTime: '2actions',
    targeting: 'hit',
  }
}

export function criterionOptions(
  criterion: SpellCriterion,
  modeIndex: number,
): readonly SpellOption[] {
  if (criterion.options) return criterion.options
  return criterion.modes?.[modeIndex]?.options ?? []
}

export function selectedOption(
  criterion: SpellCriterion,
  selection: SpellSelection,
): SpellOption | undefined {
  return criterionOptions(criterion, selection.modeIndex)[
    selection.optionIndex
  ]
}

/** Nothing picked yet: every criterion at its first option and no dice. */
export function isEmptyDraft(draft: SpellDraft): boolean {
  return (
    draft.damageDice === 0 &&
    (draft.extraEffects?.length ?? 0) === 0 &&
    (draft.extraSelfDebuffs?.length ?? 0) === 0 &&
    Object.values(draft.selections).every((s) => s.modeIndex === 0 && s.optionIndex === 0)
  )
}

/** Criteria that can be picked more than once: buffs/debuffs and self-debuffs. */
export const REPEATABLE_CRITERIA: readonly CriterionKey[] = ['buffDebuff', 'selfDebuff']

/** A repeatable criterion's picks beyond the first. */
export function extrasFor(draft: SpellDraft, key: CriterionKey): SpellSelection[] {
  if (key === 'buffDebuff') return draft.extraEffects ?? []
  if (key === 'selfDebuff') return draft.extraSelfDebuffs ?? []
  return []
}

/** The draft with a repeatable criterion's extra picks replaced. */
export function withExtras(draft: SpellDraft, key: CriterionKey, extras: SpellSelection[]): SpellDraft {
  if (key === 'buffDebuff') return { ...draft, extraEffects: extras }
  if (key === 'selfDebuff') return { ...draft, extraSelfDebuffs: extras }
  return draft
}

/** Every pick for a criterion: the first, then any extras. */
export function picksFor(draft: SpellDraft, key: CriterionKey): SpellSelection[] {
  return [selectionFor(draft, key), ...extrasFor(draft, key)]
}

/** The labels of every pick for a criterion that counts for something (tier above 0). */
export function pickLabels(
  draft: SpellDraft,
  key: CriterionKey,
  criteria: readonly SpellCriterion[] = SPELL_CRITERIA,
): string[] {
  const criterion = criteria.find((c) => c.key === key)
  if (!criterion) return []
  return picksFor(draft, key)
    .map((sel) => selectedOption(criterion, sel))
    .filter((o): o is SpellOption => o !== undefined && o.tier > 0)
    .map((o) => o.label)
}

/** The labels of every buff/debuff a draft picks. */
export function effectLabels(
  draft: SpellDraft,
  criteria: readonly SpellCriterion[] = SPELL_CRITERIA,
): string[] {
  return pickLabels(draft, 'buffDebuff', criteria)
}

/** A draft's pick for a criterion; drafts saved before it existed pick option 0. */
export function selectionFor(draft: SpellDraft, key: CriterionKey): SpellSelection {
  return draft.selections[key] ?? { modeIndex: 0, optionIndex: 0 }
}

/** EP a pick adds, or takes off (negative) for a refund criterion. */
export function criterionEp(
  criterion: SpellCriterion,
  selection: SpellSelection,
): number {
  const opt = selectedOption(criterion, selection)
  const ep = opt ? opt.tier * criterion.epPerTier : 0
  return criterion.refund ? -ep : ep
}

/** Whether a criterion applies to a draft: an `onlyWith` one needs its partner picked. */
export function criterionApplies(
  criterion: SpellCriterion,
  draft: SpellDraft,
  criteria: readonly SpellCriterion[],
): boolean {
  if (!criterion.onlyWith) return true
  const partner = criteria.find((c) => c.key === criterion.onlyWith)
  if (!partner) return false
  return pickLabels(draft, partner.key, criteria).length > 0
}

export interface SpellCost {
  baseEp: number
  multiplier: number
  totalEp: number
}

// Maneuvers price a shorter list of criteria (see maneuvers.ts); anything
// not in `criteria` costs nothing.
export function spellCost(
  draft: SpellDraft,
  criteria: readonly SpellCriterion[] = SPELL_CRITERIA,
): SpellCost {
  // Each criterion's EP over all its picks (a repeatable one has extras).
  const totals = new Map<CriterionKey, number>()
  for (const c of criteria) {
    if (!criterionApplies(c, draft, criteria) || c.multiplies) continue
    const picks = REPEATABLE_CRITERIA.includes(c.key) ? picksFor(draft, c.key) : [selectionFor(draft, c.key)]
    totals.set(c.key, picks.reduce((sum, sel) => sum + criterionEp(c, sel), 0))
  }
  // A duration that multiplies scales its criterion's total.
  for (const c of criteria) {
    if (!c.multiplies || !criterionApplies(c, draft, criteria)) continue
    const factor = selectedOption(c, selectionFor(draft, c.key))?.multiplier ?? 1
    const total = totals.get(c.multiplies)
    if (total !== undefined) totals.set(c.multiplies, total * factor)
  }
  const criteriaEp = [...totals.values()].reduce((sum, ep) => sum + ep, 0)
  const damageEp = draft.damageDice * EP_PER_DAMAGE_DIE
  // Refunds can bring a maneuver down to free, never below.
  const baseEp = Math.max(0, criteriaEp + damageEp)
  const time =
    CASTING_TIMES.find((t) => t.key === draft.castingTime) ??
    CASTING_TIMES.find((t) => t.key === '2actions')!
  const totalEp = Math.ceil(baseEp * time.multiplier)
  return { baseEp, multiplier: time.multiplier, totalEp }
}

// Saved (prepared) spells cost 25% less to cast.
export const SAVED_SPELL_DISCOUNT = 0.25

export function savedSpellCost(
  draft: SpellDraft,
  criteria: readonly SpellCriterion[] = SPELL_CRITERIA,
): SpellCost {
  const base = spellCost(draft, criteria)
  return {
    ...base,
    totalEp: Math.max(0, Math.round(base.totalEp * (1 - SAVED_SPELL_DISCOUNT))),
  }
}

export interface SavedSpell {
  id: string
  name: string
  school: string
  medium: string
  draft: SpellDraft
}

/**
 * A lasting spell needs concentration when any of its Buffs/Debuffs is one of
 * CONCENTRATION_EFFECTS, when it keeps dealing damage (or healing) dice, or
 * when it covers an area (a splash or a line).
 */
export function needsConcentration(draft: SpellDraft): boolean {
  return (
    effectLabels(draft).some((label) => CONCENTRATION_EFFECTS.has(label)) ||
    draft.damageDice > 0 ||
    isAreaSpell(draft)
  )
}

/** Covers an area (a splash or a line) rather than picking targets. */
export function isAreaSpell(draft: SpellDraft): boolean {
  const aoe = SPELL_CRITERIA.find((c) => c.key === 'aoe')
  const mode = aoe?.modes?.[selectionFor(draft, 'aoe').modeIndex]
  return mode?.key === 'splash' || mode?.key === 'line'
}

/**
 * The Buff/Debuff options that need concentration, sorted the way 5e sorts
 * its nearest spells: Guidance and Enhance Ability (+2 / +5 skill), Detect
 * Magic, Silent and Major Image, Entangle, Slow, Fear, Bestow Curse, Hold
 * Person and Monster, Silence, Banishment, Dominate, Polymorph and so on are
 * concentration. By house rule every sense is too: darkvision,
 * tremorsense, see invisibility and truesight, and detect lies. Tongues, Minor Illusion, Blindness/Deafness, Charm Person, Contagion and
 * Command are not, and neither are knowledge, raising the dead or death.
 */
export const CONCENTRATION_EFFECTS: ReadonlySet<string> = new Set([
  // Tier I
  'darkvision',
  'magic detection',
  '+2 skill',
  'inconvenience',
  'poisoned',
  'grappled',
  'slowed',
  // Tier II
  'tremorsense',
  'see invisibility',
  'detect lies',
  'multi-sense illusions',
  '+5 skill',
  'frightened',
  'exhaustion',
  // Tier III
  'truesight',
  'total evasion',
  'disability',
  'restrained',
  'silenced',
  // Tier IV
  'debilitated',
  'banished',
  'dominated',
  'polymorphed',
])

/** The duration criterion's no-concentration and concentration modes, if it has both. */
export function concentrationModes(
  criterion: SpellCriterion,
): { standard: number; concentration: number } | null {
  const modes = criterion.modes ?? []
  const concentration = modes.findIndex((m) => m.key === 'concentration')
  const standard = modes.findIndex((m) => m.key === 'standard')
  return concentration >= 0 && standard >= 0 ? { standard, concentration } : null
}

/**
 * Moves a spell's duration onto the list the concentration rule allows,
 * keeping the same length where that list has it: a concentration effect
 * or dice of damage or healing need concentration unless instantaneous;
 * otherwise no concentration.
 */
export function applyConcentrationRule(
  draft: SpellDraft,
  criteria: readonly SpellCriterion[] = SPELL_CRITERIA,
): SpellDraft {
  const duration = criteria.find((c) => c.key === 'duration')
  const modes = duration && concentrationModes(duration)
  if (!duration || !modes) return draft
  const sel = selectionFor(draft, 'duration')
  const current = selectedOption(duration, sel)
  const needs = needsConcentration(draft)
  const target = (modeIndex: number, fallback: string) => {
    const options = criterionOptions(duration, modeIndex)
    const same = options.findIndex((o) => o.label === current?.label)
    const optionIndex = same >= 0 ? same : options.findIndex((o) => o.label === fallback)
    return { modeIndex, optionIndex: Math.max(0, optionIndex) }
  }
  let next = sel
  if (needs && sel.modeIndex === modes.standard && (current?.tier ?? 0) > 0) {
    // Lasting longer than concentration can: hold it as long as possible.
    next = target(modes.concentration, '8 hours')
  } else if (!needs && sel.modeIndex === modes.concentration) {
    next = target(modes.standard, current?.label === '8 hours' ? '1 day' : '1 minute')
  }
  if (next === sel) return draft
  return { ...draft, selections: { ...draft.selections, duration: next } }
}
