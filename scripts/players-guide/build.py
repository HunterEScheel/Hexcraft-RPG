"""Builds the Hexcraft Player's Guide as guide.html; pdf.mjs prints it to
public/hexcraft-players-guide.pdf. The numbers here are copied from src/system,
so change them here too when the rules change."""
import math, pathlib

OUT = pathlib.Path(__file__).with_name('guide.html')

SKILL = [0, 2, 5, 10, 18, 31, 52, 86, 141, 230, 374, 748, 1266, 2158, 3568, 5870, 9582, 15596]
def skill(n): return SKILL[min(max(n, 0), len(SKILL) - 1)]
def ladder(n, off): return (1 if n > 0 else -1) * (skill(abs(n) + off) - skill(off)) if n else 0
attr = lambda n: ladder(n, 4)
school = lambda n: ladder(n, 4)
medium = lambda n: ladder(n, 3)

TIERS = [('Peasants', 150), ('Soldiers', 300), ('New Adventurers', 400), ('Guild Regulars', 550),
         ('Proven Hand', 700), ('Guild Veteran', 900), ('Guild Elite', 1100),
         ('National Elite', 1400), ('World Savior', 2000)]

MEDIUMS = [('Elemental', 'Fire, water, earth, lightning, air, sound.'),
           ('Magic', 'Counterspelling and effects that target magic itself.'),
           ('Kinetic', 'Manipulates speed and motion.'),
           ('Cognition', 'Charm, persuasion, and effects influencing thoughts or actions.'),
           ('Space', 'Teleportation and spatial manipulation.'),
           ('Toxicity', 'Poisons and acids.'),
           ('Material', 'Targets a specific item or person: alter self, locate object, etc.'),
           ('Vitality', 'Healing, life force, and biological vigor.')]
SCHOOLS = ['Destroy', 'Create', 'Alter', 'Restore', 'Divine', 'Control', 'Summon']

COMBAT = [('Dodge', 'Passive', '+1 Evasion per level; save vs. agility-based effects', '—'),
          ('Grit', 'Passive', 'Save vs. physical effects', '—'),
          ('Resolve', 'Passive', 'Save vs. mental effects', '—'),
          ('Parry', 'Reaction', '−1 to an incoming attack per level', '—'),
          ('1-handed melee', 'Action', 'Attack bonus with 1H melee weapons', 'Agility'),
          ('1-handed fired', 'Action', 'Attack bonus with 1H fired weapons', 'Agility'),
          ('2-handed melee', 'Action', 'Attack bonus with 2H melee weapons', 'Power'),
          ('2-handed fired', 'Action', 'Attack bonus with 2H fired weapons', 'Power'),
          ('Unarmed', 'Action', 'Attack bonus when fighting bare-handed', 'Agility'),
          ('Grapple', 'Action', 'Attack bonus to seize and hold', 'Agility'),
          ('Tackle', 'Action', 'Attack bonus to knock down or drive back', 'Power')]

RANGE = ['touch / 30 ft', '120 ft', '600 ft', '1 mile', 'any distance', 'cross-planar']
AOE = {'Targeted': [('none / self', 0), ('single target', 0), ('split damage', 1), ('3 targets', 2), ('10 targets', 3), ('100 targets', 5)],
       'Splash': [('20 ft', 1), ('60 ft', 2), ('120 ft', 3), ('300 ft', 4)],
       'Line': [('60 ft', 1), ('300 ft', 2), ('600 ft', 3), ('1 mile', 4)]}
DURATION = {'No concentration': [('instantaneous', 0), ('1 minute', 1), ('1 hour', 2), ('1 day', 3), ('1 year', 4), ('permanent', 5)],
            'Concentration': [('1 round', 1), ('1 minute', 2), ('1 hour', 3), ('8 hours', 4)]}
BUFFS = {1: 'darkvision, magic detection, languages, 1-sense illusions, +2 skill, cursory knowledge, inconvenience, poisoned, deafened, grappled, slowed, reincarnation',
         2: 'tremorsense, see invisibility, detect lies, multi-sense illusions, +5 skill, fundamental knowledge, prone, frightened, illness, undeath, exhaustion',
         3: 'truesight, total evasion, detailed knowledge, disability, restrained, blinded, silenced, charmed, resurrection',
         4: 'debilitated, banished, dominated, polymorphed, total knowledge, true resurrection',
         5: 'soul-bound, death, True Resurrection (greater)'}
CHALLENGE = [('none / 150 BP', 0), ('400 BP', 1), ('1,000 BP', 2), ('2,500 BP', 3), ('5,000 BP', 4), ('10,000 BP', 5)]
EP = {'range': 4, 'aoe': 5, 'duration': 4, 'buff': 5, 'challenge': 8}
CAST = [('Reaction', 4), ('Action', 2), ('2 Actions', 1), ('1 Minute', 0.5), ('1 Hour', 0.25)]
DAMAGE_TYPES = 'Physical, Fire, Cold, Lightning, Acid, Poison, Psychic, Magical, Force, Sonic'
ROMAN = {1: 'I', 2: 'II', 3: 'III', 4: 'IV', 5: 'V'}


def table(head, rows, cls=''):
    h = ''.join(f'<th>{c}</th>' for c in head)
    b = ''.join('<tr>' + ''.join(f'<td>{c}</td>' for c in r) + '</tr>' for r in rows)
    return f'<table class="{cls}"><thead><tr>{h}</tr></thead><tbody>{b}</tbody></table>'


def fmt(n):
    return f'{n:,}'.replace('-', '−')


def opts(pairs, per):
    return table(['Option', 'Tier', 'EP'], [(l, t, t * per) for l, t in pairs], 'compact')


costs = table(['Level', 'Skill', 'Attribute / School', 'Medium', 'Speed'],
              [(n, fmt(skill(n)), fmt(attr(n)), fmt(medium(n)), f'{20 + 5 * n} ft') for n in range(1, 9)], 'num')

sections = []
def section(id_, title, html):
    sections.append((id_, title, html))


section('welcome', 'Welcome to Hexcraft', f'''
<p class="lead">Hexcraft is a point-buy tabletop RPG with one rule at its heart: <b>roll a d20, add what you're good at, beat the number.</b></p>
<p>One player runs the world as the <b>GM</b>; everyone else plays a single character. The GM describes situations and decides when a roll is needed. You describe what your character does and roll when asked.</p>
<p>There are <b>no levels and no classes</b>. Everything your character can do is bought with <b>Build Points (BP)</b>: attributes, skills, hit points, energy, magic and even movement speed all come out of the same budget. Want a sword-swinging healer who talks her way past guards? Buy exactly that.</p>
<p>As the campaign goes on, the GM awards bonus BP and you spend it on whatever you want to grow next.</p>
<div class="callout"><b>At the table you need:</b> a d20, a few d6/d8/d10 for damage and armor, and your character sheet. Build and track your character at <b>rpg.jaeg.click</b>, which does all the arithmetic in this guide for you.</div>
''')

section('building', 'Building a Character', f'''
<h3>1. Pick a power tier</h3>
<p>The table picks a tier together. It sets everyone's starting BP. A first campaign usually starts at <b>New Adventurers</b> or <b>Guild Regulars</b>. The tier is locked once play begins; characters grow through BP awards instead.</p>
{table(['Tier', 'Starting BP'], [(n, fmt(b)) for n, b in TIERS], 'num narrow')}
<h3>2. Take tethers and flaws (optional)</h3>
<p>Tethers and flaws <b>give you extra BP</b> up front in exchange for story hooks and drawbacks. See <i>Tethers &amp; Flaws</i>.</p>
<h3>3. Spend your BP</h3>
<ul>
<li><b>Attributes</b>: Power, Agility, Intelligence, Sense, Influence</li>
<li><b>Hit Points (HP)</b>: 2 BP per 3 HP</li>
<li><b>Energy Points (EP)</b>: 2 BP per EP. EP fuels spells</li>
<li><b>Speed</b>: 20 ft by default, bought up (or sold down) in 5 ft steps</li>
<li><b>Skills</b>: the combat skills, the three saves, and any skill you can describe</li>
<li><b>Magic</b>: spell schools and mediums, if your character casts</li>
</ul>
<p>You can't spend more than your budget, and you must meet the GM's obligation threshold for tethers.</p>
<h3>Cost table</h3>
<p>Every cost in Hexcraft climbs on the same curve. Each column is the <b>total</b> BP to reach that level from 0. Attributes and speed can also go <b>below zero</b> to refund the same amounts.</p>
{costs}
<p class="small">Speed levels count 5 ft steps above 20 ft: 25 ft is level 1, 30 ft level 2, and so on. 15 ft refunds 13 BP.</p>
<div class="example"><b>Example: Mira, a New Adventurers ranger (400 BP).</b> She takes <i>Hunted by the Black Sigil</i> (Major tether, +15 BP) and <i>Trust no priest</i> (Flaw, +15 BP), giving her 430 BP. She buys Agility +3 (68), Power +1 (13), Sense +2 (34), 18 HP (12) and 10 EP (20), and spends the rest on skills, weapons and a saved spell.</div>
''')

section('abilities', 'Attributes & Skills', f'''
<h3>Attributes</h3>
{table(['Attribute', 'Covers'], [
    ('Power', 'Strength, force, two-handed weapons, tackling'),
    ('Agility', 'Speed, balance, one-handed weapons, grappling, Evasion'),
    ('Intelligence', 'Reasoning, study, memory'),
    ('Sense', 'Perception, intuition, awareness'),
    ('Influence', 'Presence, persuasion, force of personality')])}
<h3>Skills</h3>
<p>A skill is anything your character is trained in, named however you like: <i>Lockpicking</i>, <i>Court etiquette</i>, <i>Long-range rifle manufacturing</i>. Narrow skills are cheap to be great at; broad skills help more often but less (see <i>Making a Roll</i>).</p>
<p>Every character also has these <b>combat skills</b>, bought like any other skill:</p>
{table(['Skill', 'Type', 'Effect', 'Attribute'], COMBAT)}
<p class="small">Each spell school you know also counts as a skill at the school's level, listed on your sheet by its discipline: Destruction, Creation, Alteration, Restoration, Divination, Control or Summoning Magic.</p>
''')

section('tethers', 'Tethers & Flaws', f'''
<p><b>Tethers</b> are the relationships and duties that pull your character through the world: a sworn oath, a child to protect, a cult that wants you dead. <b>Flaws</b> are personal failings, physical, mental or social.</p>
<div class="two">
{table(['Tether', 'BP refund', 'Obligation weight'], [('Minor', '+5', 1), ('Major', '+15', 2), ('Binding', '+40', 3)], 'num')}
{table(['Flaw', 'BP refund'], [('Quirk', '+5'), ('Flaw', '+15'), ('Vice', '+40')], 'num')}
</div>
<p>The GM sets an <b>obligation threshold</b>: your tethers' weights must add up to at least that number. Flaws carry no weight.</p>
<p>This is a contract. When a tether or flaw costs you time, money, allies or HP, it's working as intended. If yours never come up, expect the GM to bring them in.</p>
<div class="callout"><b>For the GM: setting the threshold.</b> If you're not sure what obligation threshold to set, start at <b>3</b>. That all but guarantees every character carries obligations that matter, and that their choices will run into them.<br><br>
When a player turns their back on an obligation, make it cost them. If they abandon an alliance, the people they were allied with should strike back at the most opportune moment. The consequences should land within <b>one or two sessions</b>, while the choice is still fresh.</div>
''')

section('rolls', 'Making a Roll', f'''
<div class="formula">d20 + attribute + skill &nbsp;vs&nbsp; DC</div>
<p>The GM picks the <b>attribute</b> that fits what you're doing. You say which of your <b>skills</b> you're using for it. The GM decides how relevant that skill is and sets the <b>DC</b> from there.</p>
{table(['Difficulty', 'DC'], [('Easy', 12), ('Medium', 20), ('Hard', 30), ('Near-impossible', 40)], 'num narrow')}
<h3>How well does your skill fit?</h3>
<p>The GM starts from the task's difficulty, then adjusts it for how closely your skill matches. Round down.</p>
{table(['Fit', 'Change', 'Meaning', 'Sniper rifle (Hard, DC 30)'], [
    ('Negative', '× 1.5', 'Your training points the wrong way', 'Cooking: <b>DC&nbsp;45</b>'),
    ('None', 'unchanged', 'No skill applies', 'No skill: <b>DC&nbsp;30</b>'),
    ('Relevant', '× 0.90', 'A broad skill that touches the domain', 'Firearms knowledge: <b>DC&nbsp;27</b>'),
    ('Adjacent', '× 0.75', 'A skill in the same neighborhood', 'Firearms manufacturing: <b>DC&nbsp;22</b>'),
    ('Exact', '× 0.60', 'Precisely what you trained for', 'Long-range rifle making: <b>DC&nbsp;18</b>')])}
<h3>Natural rolls and criticals</h3>
<ul>
<li><b>Natural 20</b>: +10 to the total. <b>Natural 1</b>: −10. Neither succeeds or fails on its own; the total still has to beat the DC.</li>
<li><b>Critical success</b>: beat the DC by 10 or more. On an attack, that's a critical hit.</li>
<li><b>Critical failure</b>: total below 25% of the DC. Something goes badly wrong.</li>
</ul>
''')

section('saves', 'Saving Throws', f'''
<p>There are exactly three saves, each a combat skill you buy:</p>
{table(['Save', 'What it is', 'For example'], [
    ('Dodge', 'Avoiding', 'Leaping clear, dodging traps, weaving through fire'),
    ('Grit', 'Powering through', 'Shaking off poison, withstanding a knockdown, enduring pain'),
    ('Resolve', 'Mental resistance', 'Resisting charm, mental intrusion, illusions, fear')])}
<div class="formula">d20 + save level (+ attribute if it fits) &nbsp;vs&nbsp; DC</div>
<p>A spell's save DC is <b>10 + the caster's school + medium</b>.</p>
<div class="example"><b>Example.</b> A spider mage (school 3, medium 3) casts a charm at Mira: Resolve DC 16. Mira has Resolve 2, rolls d20 + 2 and gets 17. She resists.</div>
''')

section('combat', 'Combat', f'''
<h3>Attacking</h3>
<div class="formula">d20 + weapon skill + attribute &nbsp;vs&nbsp; Evasion</div>
<div class="formula">Evasion = 10 + Agility + Dodge − armor penalties</div>
<p>On a hit, roll the weapon's damage (for example a shortsword: 1d6 Physical). The GM may add damage types for the situation: a torch deals Fire.</p>
<div class="example"><b>Example.</b> A bandit with 1-handed melee +3 and Power +2 swings at Mira (Evasion 14): d20 + 5, rolls 13, total 18. Hit, 1d6 + 2 Physical.</div>
<h3>Damage types</h3>
<p>All damage has a type: {DAMAGE_TYPES}.</p>
<h3>Armor</h3>
{table(['Class', 'Reduction die', 'Threshold', 'Durability', 'Evasion penalty'], [
    ('Light', 'd6', 10, 12, '−2'), ('Medium', 'd8', 12, 15, '−4'), ('Heavy', 'd10', 15, 20, '−6')], 'num')}
<p>Each piece of armor protects against the damage types listed on it. Armor can be made Lightweight or Heavyweight (±1 to ±3 Evasion penalty), Durable or Brittle (±2 to ±6 threshold and durability), and more protective (+1 to +3 flat reduction).</p>
<h3>Taking damage</h3>
<ol>
<li>Find each equipped armor piece that covers the damage type and roll its reduction die. Add them up.</li>
<li>Subtract that from the damage. Temporary HP absorbs the rest first, then your HP.</li>
<li>If the damage was <b>greater than</b> a matching armor's threshold, that armor loses <b>damage ÷ threshold</b> durability (round down). At 0 it breaks and comes off.</li>
</ol>
<div class="example"><b>Example.</b> Mira's hardened leather coat (Light: d6, threshold 10, durability 12) takes a 23 Physical hit. She rolls a 4: 19 damage to HP. 23 is over 10, so the coat loses 2 durability (now 10).</div>
<p>Damage your armor doesn't cover goes straight through. Steel won't stop fire or psychic attacks.</p>
<h3>Dropping to 0 HP</h3>
<p>At 0 HP you're down and rolling <b>death saves</b> as the GM directs. <b>Three successes</b>: you're stable (unconscious, no longer dying). <b>Three failures</b>: you die. While dying you can <b>spend all your remaining EP</b> to get back up at 1 HP.</p>
<p>A <b>long rest</b> restores HP and EP to full and clears death saves.</p>
''')

aoe_rows = ''.join(f'<h4>{mode}</h4>{opts(o, EP["aoe"])}' for mode, o in AOE.items())
dur_rows = ''.join(f'<h4>{mode}</h4>{opts(o, EP["duration"])}' for mode, o in DURATION.items())
section('magic', 'Magic', f'''
<p>Every spell needs a <b>school</b> (what it does) and a <b>medium</b> (what it acts on). You must have trained both.</p>
<div class="two">
<div><h3>Schools</h3><p>{', '.join(SCHOOLS)}</p>
<p class="small">School costs follow the Attribute / School column of the cost table; medium costs follow the Medium column.</p></div>
<div><h3>Mediums</h3>{table(['Medium', 'Acts on'], MEDIUMS, 'compact')}</div>
</div>
<h3>Resolving a spell</h3>
<ul>
<li><b>Roll to hit</b>: d20 + school + medium vs the target's Evasion.</li>
<li><b>Save</b>: the target makes a Dodge, Grit or Resolve save against DC 10 + school + medium.</li>
</ul>
<h3>Amping</h3>
<p>Spend an extra <b>5 EP</b> as you cast to add +1 to the hit bonus or the save DC. Repeat as many times as you can afford.</p>
<h3>Saved spells</h3>
<p>You can save (prepare) as many spells per school as your level in that school. A saved spell costs <b>25% less EP</b> to cast.</p>
''')

section('spellbuilding', 'Building a Spell', f'''
<p>You build every spell from parts. Pick one option for each criterion, add damage dice, then pick a casting time.</p>
<div class="formula">EP = (criteria + 2 per damage die) × casting-time multiplier, rounded up</div>
<div class="two">
<div><h3>Range &middot; {EP['range']} EP per tier</h3>{opts([(r, i) for i, r in enumerate(RANGE)], EP['range'])}
<h3>Challenge &middot; {EP['challenge']} EP per tier</h3>{opts(CHALLENGE, EP['challenge'])}
<p class="small">Challenge is how strong a target the spell can overcome, measured in that target's BP.</p>
<h3>Casting time</h3>{table(['Time', 'Multiplier'], [(t, f'× {m:g}') for t, m in CAST], 'compact')}
<h3>Damage dice</h3><p><b>2 EP per die.</b></p>
</div>
<div><h3>AOE / Targets &middot; {EP['aoe']} EP per tier</h3>{aoe_rows}</div>
</div>
<div class="two">
<div><h3>Duration &middot; {EP['duration']} EP per tier</h3>{dur_rows}</div>
<div><h3>Buff / Debuff &middot; {EP['buff']} EP per tier</h3>
{table(['Tier', 'EP', 'Effects'], [(ROMAN[t], t * EP['buff'], e) for t, e in BUFFS.items()], 'compact')}</div>
</div>
<div class="example"><b>Example: Cinder Lance.</b> Devon (Destroy 3, Elemental 3) builds a single-target, 120 ft fire bolt doing 4d6, resolved as a hit roll and cast in 2 actions. Range 120 ft is 4 EP; four damage dice are 8 EP. Base cost <b>12 EP</b>; saved, <b>9 EP</b>. His hit bonus is +6; for 5 more EP (14 total) he can push it to +7.</div>
''')

MAN_DURATION = [('instantaneous', 0), ('1 round', 1), ('1 minute', 2)]
MAN_EFFECTS = {1: 'advantage on attack, +2 to a check, push 10 ft, disarmed, grappled, slowed',
               2: 'damage resistance, sneak attack, +5 to a check, prone, frightened, bleeding',
               3: 'extra action, restrained, blinded',
               4: 'debilitated'}
MAN_SELF = {1: 'rooted, exposed, off-balance',
            2: 'poisoned, burned, prone, exhaustion',
            3: 'blinded, restrained',
            4: 'debilitated'}
MAN_SELF_DURATION = [('1 round', 0), ('1 minute', 1), ('1 hour', 2), ('until a long rest', 3)]

section('maneuvers', 'Maneuvers', f'''
<p>Maneuvers are the martial side of the same system: signature strikes, sweeps, trips, disarms and feints. You build one the way you build a spell and pay for it in EP, but from a shorter list of parts:</p>
<ul>
<li><b>Range</b> and <b>AOE / targets</b>: the same options and EP as spells.</li>
<li><b>Duration</b>: instantaneous, 1 round or 1 minute. There's no concentration.</li>
<li><b>Effect</b>: non-magical effects only, listed below. There's no Challenge.</li>
<li><b>Extra damage</b>: d6s added to the weapon's own damage, 2 EP per die.</li>
<li><b>Self-debuff</b> (optional): a drawback you take on to pay for the move, which <b>takes EP off</b> its cost.</li>
<li><b>Timing</b>: the same multipliers as a spell's casting time.</li>
</ul>
<div class="two">
<div><h3>Duration &middot; {EP['duration']} EP per tier</h3>{opts(MAN_DURATION, EP['duration'])}</div>
<div><h3>Effect &middot; {EP['buff']} EP per tier</h3>
{table(['Tier', 'EP', 'Effects'], [(ROMAN[t], t * EP['buff'], e) for t, e in MAN_EFFECTS.items()], 'compact')}</div>
</div>
<h3>Self-debuffs</h3>
<p>Going all in has a price. A self-debuff refunds 5 EP per tier, and the longer it lasts the more it refunds (4 EP per tier of its own duration). The refund comes off before the timing multiplier, and a maneuver never costs less than 0.</p>
<div class="two">
<div>{table(['Tier', 'Refund', 'Self-debuffs'], [(ROMAN[t], f'−{t * 5}', e) for t, e in MAN_SELF.items()], 'compact')}</div>
<div>{table(['Lasts', 'Tier', 'Refund'], [(l, t, f'−{t * 4}' if t else '0') for l, t in MAN_SELF_DURATION], 'compact')}</div>
</div>
<p>Instead of a school and a medium, a maneuver draws on one of your <b>attack combat skills</b> (1-handed melee, 2-handed fired, Unarmed, Grapple, Tackle and so on) and that skill's attribute.</p>
<ul>
<li><b>Roll to hit</b>: d20 + combat skill + attribute vs the target's Evasion.</li>
<li><b>Save</b>: the target saves against DC 10 + combat skill + attribute.</li>
<li><b>Signature moves</b>: save as many maneuvers per combat skill as your level in it. A saved maneuver costs 25% less EP.</li>
<li><b>Amping</b> works the same: 5 more EP for +1 to hit or +1 DC.</li>
</ul>
<div class="example"><b>Example: Hamstring.</b> Mira (1-handed melee 3, Agility 3) builds a two-action cut at a single target within reach that leaves it <i>slowed</i> for 1 minute and deals her sword's damage +1d6. Slowed is a Tier I effect (5 EP), 1 minute is duration tier 2 (8 EP) and the extra die is 2 EP: <b>15 EP</b>, or <b>11 EP</b> saved. She rolls d20 + 6 against the target's Evasion.</div>
<div class="example"><b>Example: Reckless Strike.</b> Mira throws everything into one blow: advantage on the attack (5 EP) and +4d6 (8 EP), 13 EP. She plants her feet to do it and is <i>rooted</i> (−5 EP) for 1 minute (−4 EP), bringing it down to <b>4 EP</b>.</div>
''')

CONDITIONS = [
    ('Exposed', 'Anything attacking you has +5.'),
    ('Off-balance', 'Your movement speed is halved, and you add no bonuses to the rolls you make.'),
    ('Rooted', 'Your movement speed is 0.'),
    ('Poisoned', 'You take 1d6 poison damage each round.'),
    ('Burned', 'You take 1d6 fire damage each round.'),
    ('Prone', 'You fall over.'),
    ('Exhaustion', 'Per level: your HP, EP and movement speed maximums drop by 10, and all your skills, attributes and saving throws drop by 2.'),
    ('Blinded', "You can't see."),
    ('Restrained', 'You are off-balance, rooted and exposed.'),
    ('Debilitated', "You are restrained, and you can't take actions."),
    ('Slowed', 'Your movement speed is halved.'),
    ('Grappled', 'Your movement speed is 5 ft, and you have −5 to attack rolls.'),
    ('Frightened', "You can't move toward the source, attack it, or cast spells targeting it."),
    ('Bleeding', 'You lose 1d6 HP each round.'),
    ('Charmed', "You're under the effects of Cognition magic."),
]

section('conditions', 'Conditions', f'''
<p>What the conditions mean when a spell, a maneuver or a self-debuff puts them on someone.</p>
{table(['Condition', 'Effect'], CONDITIONS)}
''')

section('growing', 'Growing Your Character', f'''
<p>The GM awards bonus BP at meaningful moments: surviving a session, finishing a quest beat, uncovering a secret. The usual pace is <b>5–25 BP per session</b>. That's enough to grow steadily over a campaign; no single session is a level-up.</p>
<p>Add awarded BP in the builder's <b>BP Management</b> step, then spend it anywhere in your build. Your power tier never changes.</p>
''')

section('gm', 'For the GM', f'''
<p>The <b>Tools</b> page on the site has a DC calculator for the fit arithmetic and a spell calculator for on-the-fly casting.</p>
<ul>
<li><b>Let players pitch their skill.</b> Name the attribute, let the player say which skill they're bringing, then judge the fit honestly. A clever pitch deserves an Adjacent or Exact; a stretch is Relevant at best.</li>
<li><b>Call for the right save.</b> A cognitive trap is Resolve. A collapsing wall of flame is Dodge. A marathon climb up an icy cliff is Grit. Lean on the three saves rather than inventing new mechanics.</li>
<li><b>Use damage types.</b> Most characters armor up against Physical and Fire. Now and then, field a foe that deals Psychic, Acid or Force.</li>
<li><b>Track armor durability.</b> When armor breaks mid-fight, the stakes spike. The sheet does the math; make sure the player notices.</li>
<li><b>Reward smart spell building.</b> A caster who pays 5 EP to amp the DC is reading the odds. Tell them when it mattered.</li>
<li><b>Hold players to their tethers and flaws.</b> They were paid for in BP. If they never bite, bring them into the story.</li>
<li><b>Pace BP awards steadily.</b> 5–25 BP per session: the high end for a hard-won fight or a quest beat, the low end for quiet downtime. A new skill rank or attribute step should take a few sessions to earn.</li>
<li><b>Keep the tier fixed.</b> Grow characters with BP awards, never by raising the power tier mid-campaign.</li>
</ul>
''')


section('app', 'Using rpg.jaeg.click', f'''
<ul>
<li><b>No account needed.</b> Anyone can create a character, open its sheet and edit it.</li>
<li><b>Builder</b>: steps through name, BP, HP/EP/speed, tethers and flaws, attributes, skills and magic, with a BP bar that warns when you're over budget.</li>
<li><b>Passcode</b>: when you create a character you can give it an optional passcode. Then only someone who enters it can view, edit or delete the character. Your browser remembers it after the first time.</li>
<li><b>Sheet</b>: tracks current HP, EP and temporary HP, takes typed damage and applies armor, runs death saves and long rests, holds your inventory, saved spells and saved maneuvers, and builds spells and maneuvers on the fly.</li>
<li><b>Deleting</b> a character without a passcode needs a signed-in account.</li>
</ul>
''')

section('reference', 'Quick Reference', f'''
<div class="two">
<div>
<h3>Rolls</h3>
<p>Check: d20 + attribute + skill vs DC<br>Attack: d20 + weapon skill + attribute vs Evasion<br>Spell attack: d20 + school + medium vs Evasion<br>Spell save DC: 10 + school + medium<br>Maneuver: d20 + skill + attribute; DC 10 + skill + attribute<br>Evasion: 10 + Agility + Dodge − armor</p>
<h3>DCs</h3><p>Easy 12 &middot; Medium 20 &middot; Hard 30 &middot; Near-impossible 40</p>
<p>Fit: Negative ×1.5 &middot; Relevant ×0.9 &middot; Adjacent ×0.75 &middot; Exact ×0.6</p>
<h3>Criticals</h3><p>Nat 20: +10 &middot; Nat 1: −10<br>Crit success: beat DC by 10+<br>Crit failure: under 25% of DC</p>
</div>
<div>
<h3>Costs</h3>
<p>HP: 2 BP per 3 &middot; EP: 2 BP each<br>Speed: 20 ft base, 5 ft steps</p>
{table(['Lv', 'Skill', 'Attr/School', 'Medium'], [(n, skill(n), attr(n), medium(n)) for n in range(1, 7)], 'num compact')}
<h3>Spell EP per tier</h3>
<p>Range 4 &middot; AOE 5 &middot; Duration 4 &middot; Buff/Debuff 5 &middot; Challenge 8 &middot; Damage die 2<br>
Reaction ×4 &middot; Action ×2 &middot; 2 Actions ×1 &middot; 1 Minute ×0.5 &middot; 1 Hour ×0.25<br>
Saved spell −25% &middot; Amp: +1 per 5 EP</p>
</div>
</div>
''')

toc = ''.join(f'<li><a href="#{i}">{t}</a></li>' for i, t, _ in sections)
FLOW = {'tethers', 'saves', 'conditions', 'gm', 'app'}  # short sections run on from the previous one
body = ''.join(f'<section id="{i}" class="{"flow" if i in FLOW else ""}"><h2>{t}</h2>{h}</section>' for i, t, h in sections)

CSS = '''
@page { size: Letter; margin: 0.7in 0.75in 0.75in; }
:root { --ink:#1c1917; --muted:#57534e; --accent:#b45309; --accent-soft:#fef3c7; --rule:#e7e5e4; }
* { box-sizing: border-box; }
body { font-family: 'DejaVu Serif', 'Liberation Serif', serif; color: var(--ink); font-size: 10.5pt; line-height: 1.45; margin: 0; background: #fff; }
h1, h2, h3, h4, th, .formula, .toc, .kicker { font-family: 'DejaVu Sans', 'Liberation Sans', sans-serif; }
.cover { height: 9.4in; display: flex; flex-direction: column; justify-content: center; align-items: flex-start; page-break-after: always; border-left: 10px solid var(--accent); padding-left: 0.5in; }
.kicker { text-transform: uppercase; letter-spacing: 0.25em; color: var(--accent); font-size: 11pt; margin: 0; }
.cover h1 { font-size: 48pt; margin: 0.1in 0 0.05in; letter-spacing: -0.01em; }
.cover .sub { font-size: 16pt; color: var(--muted); margin: 0 0 0.4in; }
.hex { font-size: 64pt; color: var(--accent); line-height: 1; margin-bottom: 0.2in; }
.toc { list-style: none; padding: 0; margin: 0; columns: 2; column-gap: 0.5in; font-size: 11pt; }
.toc li { margin: 0.04in 0; } .toc a { color: var(--ink); text-decoration: none; }
section { page-break-before: always; } section.flow { page-break-before: auto; break-inside: avoid; margin-top: 0.3in; } h2, h3, h4 { break-after: avoid; }
h2 { font-size: 22pt; margin: 0 0 0.15in; padding-bottom: 0.06in; border-bottom: 3px solid var(--accent); }
h3 { font-size: 12pt; color: var(--accent); margin: 0.2in 0 0.06in; }
h4 { font-size: 9.5pt; margin: 0.1in 0 0.03in; color: var(--muted); text-transform: uppercase; letter-spacing: 0.05em; }
p { margin: 0 0 0.08in; } .lead { font-size: 12.5pt; }
ul, ol { margin: 0 0 0.1in; padding-left: 0.25in; } li { margin: 0.02in 0; }
.small { font-size: 9pt; color: var(--muted); }
table { border-collapse: collapse; width: 100%; margin: 0.06in 0 0.14in; font-size: 9.5pt; page-break-inside: avoid; }
th { text-align: left; background: var(--ink); color: #fff; font-weight: bold; padding: 4px 7px; font-size: 8.5pt; text-transform: uppercase; letter-spacing: 0.04em; }
td { padding: 3.5px 7px; border-bottom: 1px solid var(--rule); vertical-align: top; }
tbody tr:nth-child(even) td { background: #fafaf9; }
table.num td:not(:first-child), table.num th:not(:first-child) { text-align: right; }
table.narrow { width: 60%; }
table.compact { font-size: 8.8pt; } table.compact td { padding: 2.5px 6px; }
.formula { background: var(--accent-soft); border: 1px solid #fcd34d; border-radius: 6px; padding: 0.08in 0.14in; font-weight: bold; margin: 0.08in 0 0.12in; text-align: center; font-size: 11pt; }
.example { border-left: 4px solid var(--accent); background: #fafaf9; padding: 0.08in 0.14in; margin: 0.1in 0 0.14in; font-size: 9.8pt; page-break-inside: avoid; }
.callout { border: 1px solid var(--rule); border-radius: 6px; padding: 0.1in 0.14in; margin-top: 0.14in; background: #fafaf9; }
.two { display: grid; grid-template-columns: 1fr 1fr; gap: 0.25in; align-items: start; }
.two > table { margin-top: 0; }
'''

OUT.write_text(f'''<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Hexcraft Player's Guide</title><style>{CSS}</style></head><body>
<div class="cover"><div class="hex">⬡</div><p class="kicker">Tabletop Roleplaying</p><h1>Hexcraft</h1><p class="sub">Player's Guide</p>
<ul class="toc">{toc}</ul></div>
{body}</body></html>''')
print('wrote', OUT)
