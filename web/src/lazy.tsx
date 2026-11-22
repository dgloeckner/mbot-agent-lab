import type { ComponentType } from 'preact'
import { useEffect, useState } from 'preact/hooks'

/**
 * Loads a component on first render and preloads it when the browser is idle,
 * so rarely used tabs stay out of the initial bundle without feeling slow.
 */
export function lazyComponent<P extends object>(load: () => Promise<{ default: ComponentType<P> }>): ComponentType<P> {
  let loaded: ComponentType<P> | null = null
  let pending: Promise<void> | null = null
  const start = () => (pending ??= load().then((m) => void (loaded = m.default)))

  const idle = (window as { requestIdleCallback?: (cb: () => void) => void }).requestIdleCallback ?? ((cb: () => void) => setTimeout(cb, 1500))
  idle(() => void start())

  return function Lazy(props: P) {
    const [, setReady] = useState(loaded !== null)
    useEffect(() => {
      if (!loaded) void start().then(() => setReady(true))
    }, [])
    const C = loaded
    return C ? <C {...props} /> : <div class="loading" aria-busy="true" />
  }
}
