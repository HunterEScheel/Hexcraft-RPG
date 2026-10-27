import { ATTRIBUTES, type AttributeName } from './attributes'
import { COMBAT_SKILLS } from './combatSkills'
import {
  attributeCost,
  DEFAULT_SPEED,
  epCost,
  hpCost,
  magicMediumCost,
  magicSchoolCost,
  skillCost,
  speedCost,
} from './costs'
import { MAGIC_MEDIUMS, MAGIC_SCHOOLS, type MagicMedium, type MagicSchool } from './magicSchools'
import { armorEvasionReduction, type DamageType, type InventoryItem } from './inventory'
import type { CharacterSkill } from './character'
import { RANGED_MANEUVER_SKILLS, throwDistance, type ManeuverReach } from './maneuvers'
import {
  CASTING_TIMES,
  SPELL_CRITERIA,
  criterionApplies,
  pickLabels,
  selectedOption,
  selectionFor,
  targetingSummary,
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

// Built like a character's spell from a school and medium the monster knows;
// its bonus is the two levels added. Monsters have no limit on saved spells.
export interface MonsterSpell {
  id: string
  name: string
  school: MagicSchool
  medium: MagicMedium
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
  /** Non-combat skills (Stealth, Tracking (Scent), …), bought like a character's. */
  skills: CharacterSkill[]
  /** Weapons and armor; only equipped ones count. */
  inventory: InventoryItem[]
  magicSchools: Record<MagicSchool, number>
  magicMediums: Record<MagicMedium, number>
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
    skills: [],
    inventory: [],
    magicSchools: Object.fromEntries(MAGIC_SCHOOLS.map((s) => [s, 0])) as Record<MagicSchool, number>,
    magicMediums: Object.fromEntries(MAGIC_MEDIUMS.map((m) => [m, 0])) as Record<MagicMedium, number>,
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
  otherSkills: number
  magic: number
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
  const otherSkills = (m.skills ?? []).reduce((sum, s) => sum + skillCost(s.level), 0)
  const magic =
    MAGIC_SCHOOLS.reduce((sum, s) => sum + magicSchoolCost(m.magicSchools[s] ?? 0), 0) +
    MAGIC_MEDIUMS.reduce((sum, md) => sum + magicMediumCost(m.magicMediums[md] ?? 0), 0)
  const total = hp + ep + speed + attributes + skills + otherSkills + magic
  return {
    hp,
    ep,
    speed,
    attributes,
    skills,
    otherSkills,
    magic,
    total,
    remaining: m.bpBudget - total,
  }
}

/** 10 + Agility + Dodge, less what equipped armor costs, as a character's. */
export function monsterEvasion(m: Monster): number {
  const armor = (m.inventory ?? [])
    .filter((i) => i.armor && i.equipped)
    .reduce((sum, i) => sum + armorEvasionReduction(i.armor!), 0)
  return 10 + m.attributes.Agility + (m.combatSkills['combat-dodge'] ?? 0) - armor
}

export interface MonsterWeaponAttack {
  item: InventoryItem
  skillName: string
  /** The weapon's combat skill plus its attribute (a natural weapon's pick). */
  bonus: number
}

/** An attack for each equipped weapon, by the combat skill that wields it. */
export function monsterWeaponAttacks(m: Monster): MonsterWeaponAttack[] {
  return (m.inventory ?? []).flatMap((item) => {
    if (!item.equipped || !item.weaponCategory) return []
    const def = COMBAT_SKILLS.find(
      (d) => d.requiresWeapon === item.weaponCategory || d.extraWeapons === item.weaponCategory,
    )
    if (!def) return []
    const attribute = item.weaponCategory === 'natural' ? (item.attackAttribute ?? def.attribute) : def.attribute
    const bonus = (m.combatSkills[def.id] ?? 0) + (attribute ? m.attributes[attribute] : 0)
    return [{ item, skillName: def.name, bonus }]
  })
}

/** A monster spell's hit bonus: its school plus its medium (save DC is 10 + this). */
export function monsterSpellBonus(m: Monster, s: { school: MagicSchool; medium: MagicMedium }): number {
  return (m.magicSchools[s.school] ?? 0) + (m.magicMediums[s.medium] ?? 0)
}

export function monsterManeuverBonus(m: Monster, skillId: string): number {
  const def = COMBAT_SKILLS.find((c) => c.id === skillId)
  const attribute = def?.attribute ? m.attributes[def.attribute] : 0
  return (m.combatSkills[skillId] ?? 0) + attribute
}

/** How far a monster's maneuver reaches: a throw by Power, the rest by speed. */
export function monsterManeuverReach(m: Monster, skillId: string): ManeuverReach {
  if (skillId === 'combat-throw') {
    return { minRange: 0, maxRange: throwDistance(m.attributes.Power) }
  }
  // A fired maneuver goes by the first equipped weapon with a range; without
  // one it reads in multiples.
  if (RANGED_MANEUVER_SKILLS.has(skillId)) {
    const category = COMBAT_SKILLS.find((s) => s.id === skillId)?.requiresWeapon
    const w = (m.inventory ?? []).find((i) => i.equipped && i.weaponCategory === category && i.maxRange)
    return { minRange: w?.minRange, maxRange: w?.maxRange }
  }
  return { speed: m.speed }
}

export function spellTargetingLabel(s: { draft: SpellDraft }, bonus: number): string {
  return targetingSummary(s.draft.targeting ?? 'hit', bonus)
}

/**
 * The damage (or healing) dice, e.g. "4d6", or "+2d6" for a maneuver whose
 * dice add to the weapon's. Null when it rolls none.
 */
export function spellDamage(s: { draft: SpellDraft }, extraDice = false): string | null {
  const dice = s.draft.damageDice ?? 0
  return dice > 0 ? `${extraDice ? '+' : ''}${dice}d6` : null
}

// Everything but the dice, which spellDamage shows on its own.
export function spellFactors(
  s: { draft: SpellDraft },
  criteria: readonly SpellCriterion[] = SPELL_CRITERIA,
): string[] {
  const factors: string[] = []
  for (const c of criteria) {
    if (!criterionApplies(c, s.draft, criteria)) continue
    const opt = selectedOption(c, selectionFor(s.draft, c.key))
    if (!opt || (c.key === 'selfDebuff' && opt.tier === 0)) continue
    factors.push(c.key === 'selfDebuff' ? `self: ${opt.label}` : opt.label)
    for (const extra of pickLabels(s.draft, c.key, criteria).slice(opt.tier > 0 ? 1 : 0))
      factors.push(c.key === 'selfDebuff' ? `self: ${extra}` : extra)
  }
  const time = CASTING_TIMES.find((t) => t.key === s.draft.castingTime)
  if (time) factors.push(time.label)
  return factors
}
