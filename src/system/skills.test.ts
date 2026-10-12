import { describe, expect, it } from 'vitest'
import { emptyCharacter, ensureCombatSkills, skillLabel, splitSkillName } from './character'

describe('skill specificity', () => {
  it('splits "Name (Specificity)" and labels it back', () => {
    expect(splitSkillName('Animal Handling (Dogs)')).toEqual({ name: 'Animal Handling', specificity: 'Dogs' })
    expect(splitSkillName('  Lockpicking ')).toEqual({ name: 'Lockpicking' })
    expect(skillLabel({ name: 'Animal Handling', specificity: 'Dogs' })).toBe('Animal Handling (Dogs)')
    expect(skillLabel({ name: 'Lockpicking', specificity: '  ' })).toBe('Lockpicking')
  })

  it("splits older characters' typed specialties once, leaving combat skills alone", () => {
    const base = emptyCharacter('Peasants', 150)
    const old = {
      ...base,
      rulesVersion: 3,
      skills: [...base.skills, { id: 'custom-1', name: 'Driving (Motorcycle)', level: 2 }],
    }
    const once = ensureCombatSkills(old)
    const driving = once.skills.find((s) => s.id === 'custom-1')!
    expect(driving).toMatchObject({ name: 'Driving', specificity: 'Motorcycle', level: 2 })
    expect(once.skills.find((s) => s.id === 'combat-melee-1h')?.name).toBe('1-handed melee')
    expect(ensureCombatSkills(once).skills.find((s) => s.id === 'custom-1')).toEqual(driving)
  })

  it('gives a skill taken twice under one id its own id', () => {
    const base = emptyCharacter('Peasants', 150)
    const twice = {
      ...base,
      skills: [
        ...base.skills,
        { id: 'sk-12', name: 'Driving', specificity: 'Motorcycles', level: 2 },
        { id: 'sk-12', name: 'Driving', level: 1 },
      ],
    }
    const driving = ensureCombatSkills(twice).skills.filter((s) => s.name === 'Driving')
    expect(driving).toHaveLength(2)
    expect(driving[0].id).toBe('sk-12')
    expect(driving[1].id).not.toBe('sk-12')
    expect(driving[1]).toMatchObject({ name: 'Driving', level: 1 })
  })
})
