// Maneuvers are the martial counterpart to spells: built from the same
// criteria and priced in EP the same way, but drawn from an attack combat skill
// instead of a school and medium. Their bonus is that skill plus its attribute,
// as a weapon attack's is, and a character can save as many per skill as their
// level in it.
import { COMBAT_SKILLS } from './combatSkills'
import { SPELL_CRITERIA, type SpellCriterion, type SpellDraft } from './spells'

export interface SavedManeuver {
  id: string
  name: string
  /** A combat skill id, e.g. 'combat-melee-1h'. */
  skillId: string
  draft: SpellDraft
}

/** The combat skills a maneuver can be drawn from: the attacks. */
export const MANEUVER_SKILLS = COMBAT_SKILLS.filter((s) => s.category === 'action')

/** Combat skills whose maneuvers reach as far as a weapon or a throw, not a run. */
export const RANGED_MANEUVER_SKILLS: ReadonlySet<string> = new Set([
  'combat-fired-1h',
  'combat-fired-2h',
  'combat-throw',
])

/**
 * How far a maneuver can reach. Ranged and thrown ones go by the weapon's (or
 * the throw's) range in feet; the rest by the user's movement speed. Either can
 * be unknown, and the options then read in multiples instead of feet.
 */
export interface ManeuverReach {
  /** Movement speed per action, for melee and other close maneuvers. */
  speed?: number
  /** A ranged or thrown maneuver's weapon range. */
  minRange?: number
  maxRange?: number
}

/** A throw carries (Throw + Power) × 10 ft. */
export function throwDistance(throwLevel: number, power: number): number {
  return Math.max(0, (throwLevel + power) * 10)
}

const ft = (n: number) => `${n.toLocaleString('en-US')} ft`

// Range costs 0, 4, 8 then 16 EP: tier 4 skips 3 on purpose. Every maneuver's
// range and AOE share one shape, option for option and tier for tier, so a
// draft prices the same whatever reach its labels were drawn with.
function rangeCriterion(ranged: boolean, reach: ManeuverReach): SpellCriterion {
  const tiers = [0, 1, 2, 4]
  let labels: string[]
  if (ranged) {
    const max = reach.maxRange
    labels = max
      ? [
          `${reach.minRange ? `${reach.minRange.toLocaleString('en-US')}–` : 'up to '}${ft(max)}`,
          `${ft(max)}–${ft(max * 2)}`,
          `${ft(max * 2)}–${ft(max * 3)}`,
          `${ft(max * 3)}+`,
        ]
      : ['within max range', 'beyond max range', '2× max range', '3× max range']
  } else {
    const s = reach.speed
    labels = s
      ? ['no movement', `move up to ${ft(s)}`, `move up to ${ft(s * 2)}`, `move up to ${ft(s * 4)}`]
      : ['no movement', 'move up to speed', 'move up to 2× speed', 'move up to 4× speed']
  }
  return {
    key: 'range',
    label: ranged ? 'Range' : 'Movement',
    epPerTier: 4,
    options: labels.map((label, i) => ({ label, tier: tiers[i] })),
  }
}

function aoeCriterion(ranged: boolean, reach: ManeuverReach): SpellCriterion {
  const spellAoe = SPELL_CRITERIA.find((c) => c.key === 'aoe')!
  const spellMode = (key: string) => spellAoe.modes!.find((m) => m.key === key)!
  // A melee sweep reaches 5, 10 or 15 ft around; a ranged one bursts like a spell.
  const splash = ranged
    ? spellMode('splash')
    : {
        key: 'splash',
        label: 'Splash',
        options: [5, 10, 15].map((n, i) => ({ label: `${n} ft splash`, tier: i + 1 })),
      }
  // A line runs as far as the user moves (or the weapon reaches), doubling
  // each tier.
  const base = ranged ? reach.maxRange : reach.speed
  const unit = ranged ? 'range' : 'speed'
  const line = {
    key: 'line',
    label: 'Line',
    options: [1, 2, 4, 8].map((m, i) => ({
      label: base ? `${ft(base * m)} line` : `${m === 1 ? '' : `${m}× `}${unit} line`,
      tier: i + 1,
    })),
  }
  return { ...spellAoe, modes: [spellMode('targets'), splash, line] }
}

/**
 * The criteria for a maneuver drawn from this combat skill: range and AOE fit
 * to its reach, the rest the same for all.
 */
export function maneuverCriteria(skillId: string, reach: ManeuverReach = {}): readonly SpellCriterion[] {
  const ranged = RANGED_MANEUVER_SKILLS.has(skillId)
  return [rangeCriterion(ranged, reach), aoeCriterion(ranged, reach), ...SHARED_CRITERIA]
}

// The duration is short and the effects are the non-magical kind a fighter can
// pull off. There is no concentration and no Challenge. The keys match the
// spell criteria, so a maneuver is still a SpellDraft.
const SHARED_CRITERIA: readonly SpellCriterion[] = [
  // How long the effects last: multiplies their EP rather than adding.
  {
    key: 'duration',
    label: 'Duration',
    epPerTier: 0,
    multiplies: 'buffDebuff',
    onlyWith: 'buffDebuff',
    options: [
      { label: 'instantaneous', tier: 0, multiplier: 1 },
      { label: '1 round', tier: 0, multiplier: 1 },
      { label: '1 minute', tier: 0, multiplier: 2 },
      { label: '1 hour', tier: 0, multiplier: 4 },
    ],
  },
  {
    key: 'buffDebuff',
    label: 'Effect',
    epPerTier: 5,
    modes: [
      { key: 'none', label: 'None', options: [{ label: 'none', tier: 0 }] },
      {
        key: 't1',
        label: 'Tier I (5 EP)',
        options: [
          { label: 'advantage on attack', tier: 1 },
          { label: '+2 to a check', tier: 1 },
          { label: 'push 10 ft', tier: 1 },
          { label: 'disarmed', tier: 1 },
          { label: 'grappled', tier: 1 },
          { label: 'slowed', tier: 1 },
        ],
      },
      {
        key: 't2',
        label: 'Tier II (10 EP)',
        options: [
          { label: 'damage resistance', tier: 2 },
          { label: 'sneak attack', tier: 2 },
          { label: '+5 to a check', tier: 2 },
          { label: 'prone', tier: 2 },
          { label: 'frightened', tier: 2 },
          { label: 'bleeding', tier: 2 },
        ],
      },
      {
        key: 't3',
        label: 'Tier III (15 EP)',
        options: [
          { label: 'extra action', tier: 3 },
          { label: 'restrained', tier: 3 },
          { label: 'blinded', tier: 3 },
        ],
      },
      {
        key: 't4',
        label: 'Tier IV (20 EP)',
        options: [
          { label: 'debilitated', tier: 4 },
        ],
      },
    ],
  },
  // The price a fighter pays for going all in: a drawback on themselves that
  // takes EP off the maneuver, more for a worse one and for longer.
  {
    key: 'selfDebuff',
    label: 'Self-debuff',
    epPerTier: 5,
    refund: true,
    modes: [
      { key: 'none', label: 'None', options: [{ label: 'none', tier: 0 }] },
      {
        key: 't1',
        label: 'Tier I (−5 EP)',
        options: [
          { label: 'rooted', tier: 1 },
          { label: 'exposed', tier: 1 },
          { label: 'off-balance', tier: 1 },
        ],
      },
      {
        key: 't2',
        label: 'Tier II (−10 EP)',
        options: [
          { label: 'poisoned', tier: 2 },
          { label: 'burned', tier: 2 },
          { label: 'prone', tier: 2 },
          { label: 'exhaustion', tier: 2 },
        ],
      },
      {
        key: 't3',
        label: 'Tier III (−15 EP)',
        options: [
          { label: 'blinded', tier: 3 },
          { label: 'restrained', tier: 3 },
        ],
      },
      {
        key: 't4',
        label: 'Tier IV (−20 EP)',
        options: [
          { label: 'debilitated', tier: 4 },
        ],
      },
    ],
  },
  {
    key: 'selfDuration',
    label: 'Self-debuff duration',
    epPerTier: 0,
    // Multiplies every self-debuff's refund.
    multiplies: 'selfDebuff',
    onlyWith: 'selfDebuff',
    options: [
      { label: '1 round', tier: 0, multiplier: 1 },
      { label: '1 minute', tier: 0, multiplier: 2 },
      { label: '1 hour', tier: 0, multiplier: 4 },
      { label: 'until a long rest', tier: 0, multiplier: 8 },
    ],
  },
]

/**
 * Maneuver criteria with no skill or reach in mind. Ranged options are a
 * superset of the rest, so it prices any maneuver; show a skill's own
 * maneuverCriteria where the labels matter.
 */
export const MANEUVER_CRITERIA: readonly SpellCriterion[] = maneuverCriteria('combat-fired-2h')

/** Whether these criteria are a maneuver's rather than a spell's. */
export function isManeuverCriteria(criteria: readonly SpellCriterion[]): boolean {
  return criteria.some((c) => c.key === 'selfDebuff')
}
