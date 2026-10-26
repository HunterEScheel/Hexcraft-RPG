// Maneuvers are the martial counterpart to spells: built from the same
// criteria and priced in EP the same way, but drawn from an attack combat skill
// instead of a school and medium. Their bonus is that skill plus its attribute,
// as a weapon attack's is, and a character can save as many per skill as their
// level in it.
import { COMBAT_SKILLS } from './combatSkills'
import { SPELL_CRITERIA, type CastingTimeKey, type SpellCriterion, type SpellDraft } from './spells'

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

/** Body-to-body skills whose line is the user moving: a charge, a drag, a rush. */
export const MOVEMENT_MANEUVER_SKILLS: ReadonlySet<string> = new Set([
  'combat-tackle',
  'combat-grapple',
  'combat-unarmed',
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

/**
 * The actions a maneuver's timing spends, which multiply how far its line
 * runs. A no-action or reaction maneuver moves as one action does, and
 * anything a round or longer as a full round's four.
 */
export const TIMING_ACTIONS: Record<CastingTimeKey, number> = {
  none: 1,
  reaction: 1,
  action: 1,
  '2actions': 2,
  '1round': 4,
  '1minute': 4,
  '1hour': 4,
}

const ft = (n: number) => `${n.toLocaleString('en-US')} ft`

// A fired or thrown maneuver's range, by its weapon's: within it is free, then
// 4, 8 and 16 EP past twice, three and four times it (tier 4 skips 3 on
// purpose). Close maneuvers have no range: how far they go is their line.
function rangeCriterion(reach: ManeuverReach): SpellCriterion {
  const max = reach.maxRange
  const span = max ? ` (${reach.minRange ? `${reach.minRange.toLocaleString('en-US')}–` : 'up to '}${ft(max)})` : ''
  const labels = [
    `within range${span}`,
    ...[2, 3, 4].map((m) => `>${m}× max range${max ? ` (${ft(max * m)})` : ''}`),
  ]
  const tiers = [0, 1, 2, 4]
  return {
    key: 'range',
    label: 'Range',
    epPerTier: 4,
    options: labels.map((label, i) => ({ label, tier: tiers[i] })),
  }
}

// The options keep one shape across skills, tier for tier, and differ only in
// their labels: ranged splashes add a fourth.
function aoeCriterion(
  ranged: boolean,
  reach: ManeuverReach,
  actions: number,
  movement: boolean,
): SpellCriterion {
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
  // A line doubles each tier from the weapon's reach, or from as far as the
  // user moves in the actions the maneuver takes: a charge. Its EP doubles
  // too: 5, 10, 20, 40. For a tackle, grapple or punch it reads as movement,
  // the same picks and EP.
  const base = ranged ? reach.maxRange : reach.speed && reach.speed * actions
  const line = {
    key: 'line',
    label: movement ? 'Movement' : 'Line',
    options: [1, 2, 4, 8].map((m) => {
      const times = ranged ? m : m * actions
      const unit = ranged ? 'range' : 'speed'
      const reachText = base ? ft(base * m) : `${times === 1 ? '' : `${times}× `}${unit}`
      return { label: movement ? `move up to ${reachText}` : `${reachText} line`, tier: m }
    }),
  }
  return { ...spellAoe, modes: [spellMode('targets'), splash, line] }
}

/**
 * The criteria for a maneuver drawn from this combat skill: range (fired and
 * thrown only) and AOE fit to its reach and timing, the rest the same for all.
 */
export function maneuverCriteria(
  skillId: string,
  reach: ManeuverReach = {},
  castingTime: CastingTimeKey = 'action',
): readonly SpellCriterion[] {
  const ranged = RANGED_MANEUVER_SKILLS.has(skillId)
  const aoe = aoeCriterion(ranged, reach, TIMING_ACTIONS[castingTime] ?? 1, MOVEMENT_MANEUVER_SKILLS.has(skillId))
  return ranged ? [rangeCriterion(reach), aoe, ...SHARED_CRITERIA] : [aoe, ...SHARED_CRITERIA]
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
      { label: '2 rounds', tier: 0, multiplier: 1 },
      { label: '5 rounds', tier: 0, multiplier: 2 },
      { label: '1 minute', tier: 0, multiplier: 4 },
      { label: '1 hour', tier: 0, multiplier: 8 },
    ],
  },
]

/**
 * Maneuver criteria before a skill is picked. Price a saved maneuver with its
 * own skill's maneuverCriteria: close ones have no range to count.
 */
export const MANEUVER_CRITERIA: readonly SpellCriterion[] = maneuverCriteria('combat-fired-2h')

/** Whether these criteria are a maneuver's rather than a spell's. */
export function isManeuverCriteria(criteria: readonly SpellCriterion[]): boolean {
  return criteria.some((c) => c.key === 'selfDebuff')
}
