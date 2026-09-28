import { DCCalculator } from '../components/DCCalculator'

// The rules live in the Player's Guide PDF (built by scripts/players-guide);
// this page keeps only the tools a GM uses at the table.
const GUIDE_URL = '/hexcraft-players-guide.pdf'
const SHEET_URL = '/hexcraft-character-sheet.pdf'

export function RunningTheGame() {
  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-4 flex-wrap">
        <div>
          <h2 className="text-2xl font-semibold text-zinc-100">
            Running the game
          </h2>
          <p className="text-sm text-zinc-500">
            GM tools for the table. The full rules, with examples and GM tips,
            are in the Player&apos;s Guide.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <a
            href={GUIDE_URL}
            download="Hexcraft Player's Guide.pdf"
            className="rounded bg-amber-500 hover:bg-amber-400 px-3 py-1.5 text-sm font-medium text-zinc-950"
          >
            Download the Player&apos;s Guide (PDF)
          </a>
          <a
            href={SHEET_URL}
            download="Hexcraft Character Sheet.pdf"
            className="rounded bg-zinc-800 hover:bg-zinc-700 px-3 py-1.5 text-sm font-medium text-zinc-100"
          >
            Blank character sheet (PDF)
          </a>
        </div>
      </div>
      <DCCalculator />
    </div>
  )
}
