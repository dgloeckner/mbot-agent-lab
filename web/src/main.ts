// Entry point: picks the page from the URL and loads only what it needs.
//   /t/<token>  team page (editor, live log)
//   /admin      control room
//   /join       found a team with the lab code (also the fallback)
import '@fontsource-variable/bricolage-grotesque'
import './styles.css'
import { LocalStore, browserKV } from './storage'

const theme = LocalStore.loadPrefs(browserKV()).theme
if (theme !== 'system') document.documentElement.dataset.theme = theme

const el = document.getElementById('app')!
const path = location.pathname
const page = path.startsWith('/t/')
  ? import('./team/index')
  : path === '/admin' || path.startsWith('/admin/')
    ? import('./admin/index')
    : import('./join/index')
void page.then((m) => m.mount(el))
