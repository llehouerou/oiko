// Oiko's logo: its mark, the house of favicon.svg, then its name in Jost's spaced capitals.

import '@fontsource-variable/jost'

export function Mark({ className = '' }: { className?: string }) {
  return <img src="/favicon.svg" alt="" className={`size-9 shrink-0 ${className}`} />
}

// The AppBar's: a narrow screen keeps the mark only.
export function Logo() {
  return (
    <>
      <Mark className="drop-shadow-[0_4px_12px_rgba(245,158,11,0.25)]" />
      <span className="hidden font-['Jost_Variable'] text-sm font-medium tracking-[0.42em] text-neutral-200 uppercase sm:inline">Oiko</span>
    </>
  )
}
