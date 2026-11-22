import { render } from 'preact'
import '@fontsource-variable/jetbrains-mono'
import { App } from '../components/App'
import { init } from '../state'

export function mount(el: HTMLElement) {
  render(<App />, el)
  void init()
}
