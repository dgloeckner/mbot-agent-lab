import { render } from 'preact'
import '@fontsource-variable/bricolage-grotesque'
import '@fontsource-variable/jetbrains-mono'
import './styles.css'
import { App } from './components/App'
import { init } from './state'

render(<App />, document.getElementById('app')!)
void init()
