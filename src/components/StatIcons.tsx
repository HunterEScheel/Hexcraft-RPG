import type { ReactNode } from 'react'

// Icons for the headline stats: HP in a heart, EP beside a bolt, Evasion in a
// shield. The outlines are taller than a line of text, so negative margins let
// them overflow the row and keep each number level with its neighbors'.

function Outline({
  children,
  className,
  box,
  viewBox,
  path,
  nudge,
}: {
  children: ReactNode
  className: string
  /** Size and the margins that let it overflow the row. */
  box: string
  viewBox: string
  path: string
  /** Lifts the number to the widest part of the outline. */
  nudge: string
}) {
  return (
    <span className={`relative inline-flex items-center justify-center ${box} ${className}`}>
      <svg
        aria-hidden
        viewBox={viewBox}
        className="absolute inset-0 h-full w-full"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinejoin="round"
      >
        <path d={path} opacity="0.6" />
      </svg>
      <span className={`relative font-mono text-base leading-none ${nudge}`}>{children}</span>
    </span>
  )
}

/** A value inside a shield: Evasion. */
export function ShieldValue({ children, className = '' }: { children: ReactNode; className?: string }) {
  return (
    <Outline
      className={className}
      box="-mb-3 -mt-1 h-8 w-7"
      viewBox="0 0 32 36"
      path="M16 1.5 L29.5 6 V17 C29.5 25.5 23.5 31 16 34.5 C8.5 31 2.5 25.5 2.5 17 V6 Z"
      nudge="-mt-1"
    >
      {children}
    </Outline>
  )
}

/** A value inside a heart: current HP. */
export function HeartValue({ children, className = '' }: { children: ReactNode; className?: string }) {
  return (
    <Outline
      className={className}
      box="-mb-2.5 -mt-1 h-[30px] w-9"
      viewBox="0 0 34 30"
      path="M17 28 C9 22 2 16.5 2 9.5 C2 5 5.5 2 9.5 2 C12.5 2 15 3.8 17 6.5 C19 3.8 21.5 2 24.5 2 C28.5 2 32 5 32 9.5 C32 16.5 25 22 17 28 Z"
      nudge="-mt-[3px]"
    >
      {children}
    </Outline>
  )
}

/** A lightning bolt to set beside EP. */
export function BoltIcon({ className = '' }: { className?: string }) {
  return (
    <svg aria-hidden viewBox="0 0 12 18" className={`-my-px h-[17px] w-[11px] ${className}`} fill="currentColor">
      <path d="M8 0 L0 10.5 H5 L3.5 18 L12 7 H7 Z" opacity="0.85" />
    </svg>
  )
}
