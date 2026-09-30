// What the conditions mean at the table. Keyed by the option labels used in
// the spell and maneuver criteria, so the builders can show a description for
// whatever is picked. Add a condition here when a new one gets a definition.
export const CONDITIONS: Record<string, string> = {
  exposed: 'Anything attacking you has +5.',
  'off-balance': 'Your movement speed is halved, and you add no bonuses to the rolls you make.',
  rooted: 'Your movement speed is 0.',
  poisoned: 'You take 1d6 poison damage each round.',
  burned: 'You take 1d6 fire damage each round.',
  prone: 'You fall over.',
  exhaustion:
    'Per level: your HP, EP and movement speed maximums drop by 10, and all your skills, attributes and saving throws drop by 2.',
  blinded: "You can't see.",
  restrained: 'You are off-balance, rooted and exposed.',
}

export function conditionDescription(label: string): string | undefined {
  return CONDITIONS[label.toLowerCase()]
}
