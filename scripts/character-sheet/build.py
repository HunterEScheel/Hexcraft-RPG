"""Builds a blank, printable Hexcraft character sheet as sheet.html; pdf.mjs
prints it to public/hexcraft-character-sheet.pdf. Its lists (attributes, combat
skills, schools, mediums, damage types) are copied from src/system, so change
them here too when the rules change."""
import pathlib

OUT = pathlib.Path(__file__).with_name('sheet.html')

ATTRIBUTES = ['Power', 'Agility', 'Intelligence', 'Sense', 'Influence']
SAVES = ['Dodge', 'Grit', 'Resolve']
ATTACKS = [('1-handed melee', 'AGI'), ('1-handed fired', 'AGI'), ('2-handed melee', 'POW'),
           ('2-handed fired', 'POW'), ('Unarmed', 'AGI'), ('Grapple', 'AGI'), ('Tackle', 'POW')]
SCHOOLS = ['Destroy', 'Create', 'Alter', 'Restore', 'Divine', 'Control', 'Summon']
MEDIUMS = ['Elemental', 'Magic', 'Kinetic', 'Cognition', 'Space', 'Toxicity', 'Material', 'Vitality']


def field(label, cls=''):
    return f'<div class="field {cls}"><div class="line"></div><span>{label}</span></div>'


def box(label, big=False, sub=''):
    return (f'<div class="box{" big" if big else ""}"><div class="val"></div>'
            f'<span>{label}</span>{f"<em>{sub}</em>" if sub else ""}</div>')


def pool(label):
    # Current over max in one box: write current on the left, max on the right.
    return (f'<div class="box pool"><div class="val"><i>/</i></div>'
            f'<span>{label}</span><em>current / max</em></div>')


def rows(head, n, widths, cls=''):
    cols = ''.join(f'<col style="width:{w}">' for w in widths)
    h = ''.join(f'<th>{c}</th>' for c in head)
    body = ''.join('<tr>' + '<td></td>' * len(head) + '</tr>' for _ in range(n))
    return f'<table class="lines {cls}"><colgroup>{cols}</colgroup><thead><tr>{h}</tr></thead><tbody>{body}</tbody></table>'


def named(head, names, widths, cls=''):
    cols = ''.join(f'<col style="width:{w}">' for w in widths)
    h = ''.join(f'<th>{c}</th>' for c in head)
    body = ''.join('<tr>' + ''.join(f'<td>{c}</td>' for c in r) + '<td></td>' * (len(head) - len(r)) + '</tr>'
                   for r in names)
    return f'<table class="lines {cls}"><colgroup>{cols}</colgroup><thead><tr>{h}</tr></thead><tbody>{body}</tbody></table>'


def pips(n=3):
    return '<span class="pips">' + '<i></i>' * n + '</span>'


def panel(title, html, cls=''):
    return f'<section class="panel {cls}"><h3>{title}</h3>{html}</section>'


WEAPON_ROWS = 13
INVENTORY_ROWS = 40

page1 = f'''
<header class="top">
  <div class="brand"><b>⬡ Hexcraft</b><span>Character Sheet</span></div>
  <div class="idrow">{field('Character name', 'w3')}{field('Player')}</div>
  <div class="idrow">{field('Total BP spent')}{field('Total BP remaining')}</div>
</header>

<div class="stats">
  {pool('HP')}{box('Temp HP', True)}{pool('EP')}
  {box('Speed', True, 'ft')}{box('Evasion', True, '10 + AGI + Dodge − armor')}
</div>

<div class="grid2">
  {panel('Attributes', named(['Attribute', 'Level', 'BP'], [(a,) for a in ATTRIBUTES], ['56%', '22%', '22%']))}
  {panel('Saves', named(['Save', 'Level', 'BP'], [('Dodge <small>avoiding</small>',), ('Grit <small>powering through</small>',), ('Resolve <small>mental resistance</small>',)], ['56%', '22%', '22%']) +
         '<div class="deaths"><b>Death saves</b><span>Successes ' + pips() + '</span><span>Failures ' + pips() + '</span></div>')}
</div>

{panel('Combat skills', named(['Skill', 'Attr', 'Level', 'Attack bonus', 'BP'],
       [(n, a) for n, a in ATTACKS] + [('Parry <small>−1 to an incoming attack per level</small>', '—')],
       ['40%', '10%', '14%', '20%', '16%']))}

<div class="grid2">
  {panel('Weapons', rows(['Weapon', 'Type', 'Damage', 'Damage types'], WEAPON_ROWS, ['34%', '18%', '18%', '30%']))}
  {panel('Armor', rows(['Armor', 'Die', 'Reduces', 'Thr', 'Dur', 'Eva', 'On'], WEAPON_ROWS, ['27%', '9%', '28%', '9%', '11%', '8%', '8%']) +
         '<p class="hint">Die: light d6 · medium d8 · heavy d10. Damage over the threshold costs damage ÷ threshold durability.</p>')}
</div>

'''

page2 = f'''
<div class="grid2 tall">
  {panel('Skills', rows(['Skill', 'Specificity', 'Level', 'BP'], 23, ['40%', '32%', '14%', '14%']))}
  <div class="stack">
    {panel('Magic', '<div class="grid2 tight">' +
           named(['School', 'Lv'], [(s,) for s in SCHOOLS], ['72%', '28%'], 'compact') +
           named(['Medium', 'Lv'], [(m,) for m in MEDIUMS], ['72%', '28%'], 'compact') +
           '</div><p class="hint">Spell attack: d20 + school + medium vs Evasion. Save DC: 10 + school + medium.</p>')}
    {panel('Tethers', rows(['Tether', 'Tier', 'Weight'], 4, ['66%', '17%', '17%']) +
           '<div class="inline">' + field('Obligation threshold') + field('Total weight') + '</div>')}
    {panel('Flaws', rows(['Flaw', 'Quirk / Flaw / Vice'], 3, ['66%', '34%']))}
  </div>
</div>

{panel('Saved spells', rows(['Spell', 'School', 'Medium', 'Range', 'AOE', 'Duration', 'Effect', 'Bonus/DC', 'EP'], 8,
       ['18%', '9%', '9%', '9%', '9%', '9%', '22%', '9%', '6%'], 'compact'))}

{panel('Saved maneuvers', rows(['Maneuver', 'Combat skill', 'Range', 'AOE', 'Duration', 'Effect', 'Self-debuff', 'Bonus/DC', 'EP'], 7,
       ['16%', '14%', '8%', '8%', '8%', '17%', '15%', '8%', '6%'], 'compact'))}

'''

page3 = f'''
{panel('Inventory', '<div class="grid2 tight">' + rows(['Item', 'Qty', 'Notes'], INVENTORY_ROWS, ['50%', '14%', '36%']) + rows(['Item', 'Qty', 'Notes'], INVENTORY_ROWS, ['50%', '14%', '36%']) + '</div><div class="inline">' + field('Gold') + '<div class="field w3"></div></div>')}
'''

CSS = '''
@page { size: Letter; margin: 0.4in; }
:root { --ink:#1c1917; --muted:#78716c; --rule:#a8a29e; --soft:#e7e5e4; --accent:#b45309; }
* { box-sizing: border-box; }
body { margin:0; font-family:'DejaVu Sans','Liberation Sans',sans-serif; color:var(--ink); font-size:8pt; background:#fff; }
.page { height: 10.15in; display:flex; flex-direction:column; gap:0.1in; page-break-after:always; overflow:hidden; }
.page:last-child { page-break-after:auto; }
.top { border-bottom:2px solid var(--ink); padding-bottom:0.06in; }
.brand { display:flex; align-items:baseline; gap:0.12in; margin-bottom:0.04in; }
.brand b { font-size:16pt; color:var(--accent); } .brand span { font-size:10pt; text-transform:uppercase; letter-spacing:0.2em; color:var(--muted); }
.idrow { display:flex; gap:0.14in; margin-top:0.05in; }
.field { flex:1; } .field.w3 { flex:3; }
.field .line { border-bottom:1px solid var(--ink); height:0.24in; }
.field span, .box span, th { font-size:6.5pt; text-transform:uppercase; letter-spacing:0.06em; color:var(--muted); }
.stats { display:grid; grid-template-columns:1.6fr 1fr 1.6fr 1fr 1fr; gap:0.08in; }
.pool .val { display:flex; align-items:center; justify-content:center; }
.pool .val i { font-style:normal; font-size:22pt; color:var(--rule); font-weight:200; }
.box { border:1.5px solid var(--ink); border-radius:6px; padding:0.03in 0.05in; text-align:center; }
.box .val { height:0.36in; }
.box em { display:block; font-style:normal; font-size:5.5pt; color:var(--muted); }
.grid2 { display:grid; grid-template-columns:1fr 1fr; gap:0.12in; align-items:start; }
.grid2.tight { gap:0.08in; }
.panel { border:1.5px solid var(--ink); border-radius:6px; padding:0.05in 0.07in 0.07in; }
.panel h3 { margin:0 0 0.04in; font-size:8.5pt; text-transform:uppercase; letter-spacing:0.12em; color:var(--accent); }
.stack > .panel + .panel { margin-top:0.1in; }
table.lines { width:100%; border-collapse:collapse; table-layout:fixed; }
table.lines th { text-align:left; padding:0 3px 2px; border-bottom:1px solid var(--ink); font-weight:normal; }
table.lines td { height:0.225in; border-bottom:1px solid var(--rule); padding:0 3px; font-size:8pt; white-space:nowrap; overflow:hidden; }
table.lines td + td, table.lines th + th { border-left:1px solid var(--soft); }
table.compact td { height:0.22in; font-size:7.5pt; }
td small { color:var(--muted); font-size:6pt; margin-left:3px; }
.deaths { display:flex; gap:0.16in; align-items:center; margin-top:0.07in; font-size:7.5pt; }
.pips { display:inline-flex; gap:3px; vertical-align:middle; margin-left:3px; }
.pips i { width:10px; height:10px; border:1.3px solid var(--ink); border-radius:50%; display:inline-block; }
.hint { margin:0.04in 0 0; font-size:6.3pt; color:var(--muted); }
.inline { display:flex; gap:0.14in; margin-top:0.06in; }
.notes { margin-bottom:0.05in; } .notes b { font-size:6.5pt; text-transform:uppercase; letter-spacing:0.06em; color:var(--muted); font-weight:normal; }
.lines-blank { height:0.5in; background:repeating-linear-gradient(to bottom, transparent 0, transparent 0.24in, var(--rule) 0.24in, var(--rule) 0.25in); }
.lines-blank.tall { height:0.75in; }
'''

OUT.write_text(f'''<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Hexcraft Character Sheet</title><style>{CSS}</style></head><body>
<div class="page">{page1}</div><div class="page">{page2}</div><div class="page">{page3}</div></body></html>''')
print('wrote', OUT)
