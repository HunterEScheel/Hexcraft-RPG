// Prints sheet.html (from build.py) to public/hexcraft-character-sheet.pdf with
// Chromium via Playwright (not a project dependency: `npm i --no-save playwright`
// first if it is not installed). Run from the repo root:
//   python3 scripts/character-sheet/build.py && node scripts/character-sheet/pdf.mjs
import { chromium } from 'playwright';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const browser = await chromium.launch();
const page = await browser.newPage();
await page.goto('file://' + resolve(here, 'sheet.html'));
await page.pdf({
  path: resolve(here, '../../public/hexcraft-character-sheet.pdf'),
  format: 'Letter',
  printBackground: true,
  preferCSSPageSize: true,
});
await browser.close();
