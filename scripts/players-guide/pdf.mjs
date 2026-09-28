// Prints guide.html (from build.py) to public/hexcraft-players-guide.pdf with
// Chromium via Playwright (not a project dependency: `npm i --no-save playwright`
// first if it is not installed). Run from the repo root:
//   python3 scripts/players-guide/build.py && node scripts/players-guide/pdf.mjs
import { chromium } from 'playwright';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const browser = await chromium.launch();
const page = await browser.newPage();
await page.goto('file://' + resolve(here, 'guide.html'));
await page.pdf({
  path: resolve(here, '../../public/hexcraft-players-guide.pdf'),
  format: 'Letter',
  printBackground: true,
  preferCSSPageSize: true,
  displayHeaderFooter: true,
  headerTemplate: '<span></span>',
  footerTemplate:
    '<div style="width:100%;font-family:DejaVu Sans,sans-serif;font-size:8px;color:#78716c;padding:0 0.75in;display:flex;justify-content:space-between"><span>Hexcraft · Player\'s Guide</span><span class="pageNumber"></span></div>',
});
await browser.close();
