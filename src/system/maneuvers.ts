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

// A maneuver prices range and targets the way a spell does, but its duration
// is short and its effects are the non-magical kind a fighter can pull off.
// There is no concentration and no Challenge. The keys match the spell
// criteria, so a maneuver is still a SpellDraft.
export const MANEUVER_CRITERIA: readonly SpellCriterion[] = [
  SPELL_CRITERIA.find((c) => c.key === 'range')!,
  SPELL_CRITERIA.find((c) => c.key === 'aoe')!,
  {
    key: 'duration',
    label: 'Duration',
    epPerTier: 4,
    options: [
      { label: 'instantaneous', tier: 0 },
      { label: '1 round', tier: 1 },
      { label: '1 minute', tier: 2 },
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
          { label: 'stunned', tier: 4 },
          { label: 'incapacitated', tier: 4 },
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
          { label: 'flanked', tier: 1 },
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
          { label: 'stunned', tier: 4 },
          { label: 'incapacitated', tier: 4 },
        ],
      },
    ],
  },
  {
    key: 'selfDuration',
    label: 'Self-debuff duration',
    epPerTier: 4,
    refund: true,
    onlyWith: 'selfDebuff',
    options: [
      { label: '1 round', tier: 0 },
      { label: '1 minute', tier: 1 },
      { label: '1 hour', tier: 2 },
      { label: 'until a long rest', tier: 3 },
    ],
  },
]
