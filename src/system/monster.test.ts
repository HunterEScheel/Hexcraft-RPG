import { describe, expect, it } from 'vitest'
import { newArmorStats } from './inventory'
import { emptyMonster, monsterEvasion, monsterManeuverReach, monsterWeaponAttacks } from './monster'

describe('monster weapons and armor', () => {
  const ogre = () => {
    const m = emptyMonster('New Adventurers', 2000)
    m.attributes.Power = 3
    m.attributes.Agility = 1
    m.combatSkills['combat-melee-2h'] = 2
    m.combatSkills['combat-fired-2h'] = 1
    m.combatSkills['combat-unarmed'] = 1
    return m
  }

  it('lose Evasion to equipped armor only', () => {
    const m = ogre()
    const base = monsterEvasion(m)
    m.inventory = [{ id: 'a', name: 'Hide', quantity: 1, armor: { ...newArmorStats(), class: 'medium' }, equipped: false }]
    expect(monsterEvasion(m)).toBe(base)
    m.inventory[0].equipped = true
    expect(monsterEvasion(m)).toBe(base - 4)
  })

  it('attack with each equipped weapon by its combat skill', () => {
    const m = ogre()
    m.inventory = [
      { id: 'c', name: 'Club', quantity: 1, weaponCategory: 'melee-2h', equipped: true },
      { id: 'k', name: 'Knife', quantity: 1, weaponCategory: 'melee-1h', equipped: false },
      { id: 'h', name: 'Horns', quantity: 1, weaponCategory: 'natural', attackAttribute: 'Power', equipped: true },
    ]
    expect(monsterWeaponAttacks(m).map((a) => [a.item.name, a.skillName, a.bonus])).toEqual([
      ['Club', '2-handed melee', 5],
      ['Horns', 'Unarmed', 4],
    ])
  })

  it('give fired maneuvers the equipped weapon\'s range', () => {
    const m = ogre()
    expect(monsterManeuverReach(m, 'combat-fired-2h')).toEqual({ minRange: undefined, maxRange: undefined })
    m.inventory = [{ id: 'r', name: 'Rifle', quantity: 1, weaponCategory: 'fired-2h', equipped: true, minRange: 30, maxRange: 800 }]
    expect(monsterManeuverReach(m, 'combat-fired-2h')).toEqual({ minRange: 30, maxRange: 800 })
  })
})
