// Small inline icons; stroke follows currentColor.

const base = { width: 16, height: 16, viewBox: '0 0 16 16', fill: 'none', stroke: 'currentColor', 'stroke-width': 1.6, 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'aria-hidden': true } as const

export const StopIcon = () => (
  <svg {...base}>
    <rect x="3.5" y="3.5" width="9" height="9" rx="1.5" fill="currentColor" stroke="none" />
  </svg>
)

export const PlayIcon = () => (
  <svg {...base}>
    <path d="M5 3.5v9l7-4.5z" fill="currentColor" stroke="none" />
  </svg>
)

export const SunIcon = () => (
  <svg {...base}>
    <circle cx="8" cy="8" r="3" />
    <path d="M8 1.5v1.5M8 13v1.5M1.5 8H3M13 8h1.5M3.4 3.4l1 1M11.6 11.6l1 1M3.4 12.6l1-1M11.6 4.4l1-1" />
  </svg>
)

export const MoonIcon = () => (
  <svg {...base}>
    <path d="M13 9.5A5.5 5.5 0 0 1 6.5 3a5.5 5.5 0 1 0 6.5 6.5z" />
  </svg>
)

export const AutoIcon = () => (
  <svg {...base}>
    <circle cx="8" cy="8" r="5.5" />
    <path d="M8 2.5v11" />
    <path d="M8 2.5a5.5 5.5 0 0 1 0 11z" fill="currentColor" stroke="none" />
  </svg>
)

export const SendIcon = () => (
  <svg {...base}>
    <path d="M2.5 8h9M8 4l4 4-4 4" />
  </svg>
)

export const ResetIcon = () => (
  <svg {...base}>
    <path d="M3 8a5 5 0 1 0 1.5-3.6" />
    <path d="M3 2.5v2.5h2.5" />
  </svg>
)
