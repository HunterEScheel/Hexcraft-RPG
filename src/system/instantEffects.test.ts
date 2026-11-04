import { describe, expect, it } from 'vitest'
import {
  SPELL_CRITERIA,
  applyDurationRules,
  criterionApplies,
  emptySpellDraft,
  selectedOption,
  spellCost,
  spellDurationLocked,
  type SpellCriterion,
  type SpellDraft,
  type SpellSelection,
} from './spells'
import { maneuverCriteria } from './maneuvers'
import { conditionDescription } from './conditions'

// The selection that picks this label in a criterion.
function sel(criteria: readonly SpellCriterion[], key: string, label: string): SpellSelection {
  const c = criteria.find((x) => x.key === key)!
  if (c.options) return { modeIndex: 0, optionIndex: c.options.findIndex((o) => o.label === label) }
  const modeIndex = c.modes!.findIndex((m) => m.options.some((o) => o.label === label))
  return { modeIndex, optionIndex: c.modes![modeIndex].options.findIndex((o) => o.label === label) }
}

const durationLabel = (d: SpellDraft, criteria: readonly SpellCriterion[] = SPELL_CRITERIA) =>
  selectedOption(criteria.find((c) => c.key === 'duration')!, d.selections.duration)?.label

describe('instant effects (prone, exhaustion)', () => {
  it('lock a spell with only those effects to instantaneous', () => {
    const d = emptySpellDraft()
    d.selections.buffDebuff = sel(SPELL_CRITERIA, 'buffDebuff', 'prone')
    d.extraEffects = [sel(SPELL_CRITERIA, 'buffDebuff', 'exhaustion')]
    d.selections.duration = sel(SPELL_CRITERIA, 'duration', '1 day')
    expect(spellDurationLocked(d)).toBe(true)
    const locked = applyDurationRules(d)
    expect(durationLabel(locked)).toBe('instantaneous')
    // prone 10 + exhaustion 10, and no duration EP.
    expect(spellCost(locked).baseEp).toBe(20)
  })

  it("don't lock a spell that has a lasting effect or dice", () => {
    const d = emptySpellDraft()
    d.selections.buffDebuff = sel(SPELL_CRITERIA, 'buffDebuff', 'prone')
    d.extraEffects = [sel(SPELL_CRITERIA, 'buffDebuff', 'charmed')]
    expect(spellDurationLocked(d)).toBe(false)
    expect(spellDurationLocked({ ...d, extraEffects: [], damageDice: 2 })).toBe(false)
    // No effects at all is not locked either.
    expect(spellDurationLocked(emptySpellDraft())).toBe(false)
  })

  it('are never multiplied by a maneuver duration', () => {
    const criteria = maneuverCriteria('combat-melee-1h')
    const d = { ...emptySpellDraft(), castingTime: 'action' as const }
    d.selections.buffDebuff = sel(criteria, 'buffDebuff', 'prone')
    d.selections.duration = sel(criteria, 'duration', '1 hour') // ×4
    const duration = criteria.find((c) => c.key === 'duration')!
    // Nothing lasts, so there's no duration to pick.
    expect(criterionApplies(duration, d, criteria)).toBe(false)
    expect(spellCost(d, criteria).baseEp).toBe(10)
    // With a lasting effect beside it, only that one is multiplied: 10 + 10 × 4.
    d.extraEffects = [sel(criteria, 'buffDebuff', 'frightened')]
    expect(criterionApplies(duration, d, criteria)).toBe(true)
    expect(spellCost(d, criteria).baseEp).toBe(50)
  })

  it('refund a self-debuff once, whatever its duration', () => {
    const criteria = maneuverCriteria('combat-melee-1h')
    const d = { ...emptySpellDraft(), castingTime: 'action' as const, damageDice: 20 } // 40 EP
    d.selections.selfDebuff = sel(criteria, 'selfDebuff', 'prone') // −10
    d.selections.selfDuration = sel(criteria, 'selfDuration', 'until a long rest') // ×12
    const selfDuration = criteria.find((c) => c.key === 'selfDuration')!
    expect(criterionApplies(selfDuration, d, criteria)).toBe(false)
    expect(spellCost(d, criteria).baseEp).toBe(30)
    // A lasting self-debuff beside it is multiplied; prone still isn't: 40 − 10 − 5 × 12.
    d.extraSelfDebuffs = [sel(criteria, 'selfDebuff', 'rooted')]
    expect(criterionApplies(selfDuration, d, criteria)).toBe(true)
    expect(spellCost(d, criteria).baseEp).toBe(0)
  })

  it('describe prone as rooted, exposed and off-balance in melee, half cover at range', () => {
    expect(conditionDescription('prone')).toBe(
      'You are rooted. In melee you are exposed and off-balance; against ranged attacks you have half cover. You can use 1 action to remove this condition.',
    )
  })
})
