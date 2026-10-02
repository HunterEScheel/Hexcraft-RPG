import { ATTRIBUTES, type AttributeName } from './attributes'
import { COMBAT_SKILLS } from './combatSkills'
import {
  attributeCost,
  DEFAULT_SPEED,
  epCost,
  hpCost,
  skillCost,
  speedCost,
} from './costs'
import type { DamageType } from './inventory'
import {
  CASTING_TIMES,
  SPELL_CRITERIA,
  criterionApplies,
  selectedOption,
  selectionFor,
  type SpellCriterion,
  type SpellDraft,
} from './spells'

export interface MonsterAttack {
  id: string
  name: string
  hitBonus: number
  damage: string
  damageType: DamageType
  note: string
}

export interface MonsterSpell {
  id: string
  name: string
  draft: SpellDraft
}

// Built like a character's maneuver, and drawn from one of the monster's
// attack skills: its bonus is that skill plus the skill's attribute.
export interface MonsterManeuver {
  id: string
  name: string
  skillId: string
  draft: SpellDraft
}

export interface LairAction {
  id: string
  name: string
  description: string
}

export interface LegendaryAction {
  id: string
  name: string
  cost: number
  description: string
}

export interface Monster {
  name: string
  tierName: string
  bpBudget: number
  hp: number
  ep: number
  speed: number
  attributes: Record<AttributeName, number>
  combatSkills: Record<string, number>
  spellBonus: number
  multiattack: string
  attacks: MonsterAttack[]
  spells: MonsterSpell[]
  maneuvers: MonsterManeuver[]
  lairActions: LairAction[]
  legendaryActionSlots: number
  legendaryActions: LegendaryAction[]
}

export function emptyMonster(tierName: string, bpBudget: number): Monster {
  return {
    name: '',
    tierName,
    bpBudget,
    hp: 0,
    ep: 0,
    speed: DEFAULT_SPEED,
    attributes: Object.fromEntries(ATTRIBUTES.map((a) => [a, 0])) as Record<
      AttributeName,
      number
    >,
    combatSkills: Object.fromEntries(COMBAT_SKILLS.map((c) => [c.id, 0])),
    spellBonus: 0,
    multiattack: '',
    attacks: [],
    spells: [],
    maneuvers: [],
    lairActions: [],
    legendaryActionSlots: 0,
    legendaryActions: [],
  }
}

export interface MonsterBPBreakdown {
  hp: number
  ep: number
  speed: number
  attributes: number
  skills: number
  total: number
  remaining: number
}

export function monsterBpBreakdown(m: Monster): MonsterBPBreakdown {
  const hp = hpCost(m.hp)
  const ep = epCost(m.ep)
  const speed = speedCost(m.speed)
  const attributes = ATTRIBUTES.reduce(
    (sum, a) => sum + attributeCost(m.attributes[a]),
    0,
  )
  const skills = Object.values(m.combatSkills).reduce(
    (sum, lv) => sum + skillCost(lv),
    0,
  )
  const total = hp + ep + speed + attributes + skills
  return {
    hp,
    ep,
    speed,
    attributes,
    skills,
    total,
    remaining: m.bpBudget - total,
  }
}

export function monsterEvasion(m: Monster): number {
  return 10 + m.attributes.Agility + (m.combatSkills['combat-dodge'] ?? 0)
}

export function monsterManeuverBonus(m: Monster, skillId: string): number {
  const def = COMBAT_SKILLS.find((c) => c.id === skillId)
  const attribute = def?.attribute ? m.attributes[def.attribute] : 0
  return (m.combatSkills[skillId] ?? 0) + attribute
}

export function spellTargetingLabel(s: { draft: SpellDraft }, bonus: number): string {
  const targeting = s.draft.targeting ?? 'hit'
  if (targeting === 'hit') return `hit ${bonus >= 0 ? '+' : ''}${bonus}`
  const save =
    targeting === 'dodge' ? 'Dodge' : targeting === 'grit' ? 'Grit' : 'Resolve'
  return `${save} DC ${10 + bonus}`
}

// Maneuvers pass their own criteria; their dice add to the weapon's damage.
export function spellFactors(
  s: { draft: SpellDraft },
  criteria: readonly SpellCriterion[] = SPELL_CRITERIA,
  extraDice = false,
): string[] {
  const factors: string[] = []
  for (const c of criteria) {
    if (!criterionApplies(c, s.draft, criteria)) continue
    const opt = selectedOption(c, selectionFor(s.draft, c.key))
    if (!opt || (c.key === 'selfDebuff' && opt.tier === 0)) continue
    factors.push(c.key === 'selfDebuff' ? `self: ${opt.label}` : opt.label)
  }
  if (s.draft.damageDice > 0)
    factors.push(`${extraDice ? '+' : ''}${s.draft.damageDice}d6`)
  const time = CASTING_TIMES.find((t) => t.key === s.draft.castingTime)
  if (time) factors.push(time.label)
  return factors
}
