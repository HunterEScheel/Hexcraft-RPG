import type { ReactNode } from 'react'

/**
 * A stat value sitting inside a shield outline: Evasion. Negative margins let
 * it overflow the row so its number lines up with its neighbors'.
 */
export function ShieldValue({ children, className = '' }: { children: ReactNode; className?: string }) {
  return (
    <span className={`relative -mb-3 -mt-1 inline-flex h-8 w-7 items-center justify-center ${className}`}>
      <svg
        aria-hidden
        viewBox="0 0 32 36"
        className="absolute inset-0 h-full w-full"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinejoin="round"
      >
        <path d="M16 1.5 L29.5 6 V17 C29.5 25.5 23.5 31 16 34.5 C8.5 31 2.5 25.5 2.5 17 V6 Z" opacity="0.6" />
      </svg>
      <span className="relative -mt-1 font-mono text-base leading-none">{children}</span>
    </span>
  )
}
