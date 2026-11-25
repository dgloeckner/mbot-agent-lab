// A lab for browser tests: the built labserver with the scripted model and a
// temp database, simulated robots over real MQTT, and small API helpers for
// the control room and the teams. Mirrors sim/e2e/test_smoke.py.

import { spawn, type ChildProcess } from 'node:child_process'
import { mkdtempSync, openSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { createServer } from 'node:net'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'

export const ROOT = resolve(__dirname, '..', '..')
const BINARY = process.env.LABSERVER ?? join(ROOT, 'bin', 'labserver')
export const SHOTS = join(ROOT, 'web', 'e2e', 'screenshots')

async function freePort(): Promise<number> {
  return new Promise((res, rej) => {
    const s = createServer()
    s.listen(0, '127.0.0.1', () => {
      const port = (s.address() as { port: number }).port
      s.close(() => res(port))
    })
    s.on('error', rej)
  })
}

export interface Team {
  id: string
  name: string
  token: string
}

export class Lab {
  base = ''
  readonly adminPassword = 'e2e-' + Math.random().toString(36).slice(2, 10)
  readonly labCode = 'WEB1'
  private procs: ChildProcess[] = []
  private tmp = mkdtempSync(join(tmpdir(), 'lab-web-e2e-'))
  private cookie = ''
  serverLog = join(this.tmp, 'server.log')
  simLog = join(this.tmp, 'sim.log')

  /** Starts server and robots; `world` is a file under sim/worlds. */
  static async start(opts: { robots?: number; world?: string } = {}): Promise<Lab> {
    const lab = new Lab()
    await lab.boot(opts.robots ?? 1, opts.world ?? 'arena.json')
    return lab
  }

  private async boot(robots: number, world: string) {
    const [http, mqtt] = [await freePort(), await freePort()]
    this.base = `http://127.0.0.1:${http}`
    let cfg = readFileSync(join(ROOT, 'deploy', 'local', 'lab.yaml'), 'utf8')
    cfg = cfg.replace(/http_addr: "[^"]*"/, `http_addr: "127.0.0.1:${http}"`)
    cfg = cfg.replace(/mqtt_plain_addr: "[^"]*"/, `mqtt_plain_addr: "127.0.0.1:${mqtt}"`)
    cfg = cfg.replace(/db_path: \S+/, `db_path: ${JSON.stringify(join(this.tmp, 'lab.db'))}`)
    writeFileSync(join(this.tmp, 'lab.yaml'), cfg)
    const env: NodeJS.ProcessEnv = { ...process.env, LAB_ADMIN_PASSWORD: this.adminPassword, LAB_CODE: this.labCode }
    for (const n of [1, 2, 3]) env[`MBOT${n}_PASSWORD`] = 'e2e'
    for (const m of cfg.matchAll(/api_key_env:\s*(\w+)/g)) delete env[m[1]]
    this.spawn(BINARY, ['-config', join(this.tmp, 'lab.yaml')], env, this.serverLog)
    await this.until('server /healthz', async () => (await fetch(this.base + '/healthz').catch(() => null))?.ok === true)
    this.spawn(
      process.env.PYTHON ?? 'python3',
      [join(ROOT, 'sim', 'run.py'), '--robots', String(robots), '--port', String(mqtt), '--world', join(ROOT, 'sim', 'worlds', world), '--trace', '0.5', '--seed', '1234'],
      env,
      this.simLog,
    )
    await this.adminCall('POST', '/admin/api/login', { password: this.adminPassword })
    for (let i = 1; i <= robots; i++) await this.robotOnline(`mbot-${i}`)
  }

  private spawn(cmd: string, args: string[], env: NodeJS.ProcessEnv, log: string) {
    const fd = openSync(log, 'a')
    const p = spawn(cmd, args, { env, cwd: this.tmp, stdio: ['ignore', fd, fd], detached: true })
    this.procs.push(p)
  }

  async stop() {
    for (const p of [...this.procs].reverse()) {
      try {
        process.kill(-p.pid!, 'SIGTERM')
      } catch {
        /* already gone */
      }
    }
    await new Promise((r) => setTimeout(r, 300))
    rmSync(this.tmp, { recursive: true, force: true })
  }

  /** Polls until probe returns something truthy (20 s). */
  async until<T>(what: string, probe: () => Promise<T | null | undefined | false>, ms = 20_000): Promise<T> {
    const deadline = Date.now() + ms
    for (;;) {
      for (const p of this.procs) {
        if (p.exitCode !== null) throw new Error(`process ${p.spawnfile} exited with ${p.exitCode} while waiting for ${what}\n${this.logs()}`)
      }
      const got = await probe().catch(() => null)
      if (got) return got as T
      if (Date.now() > deadline) throw new Error(`timeout waiting for ${what}\n${this.logs()}`)
      await new Promise((r) => setTimeout(r, 100))
    }
  }

  logs(): string {
    const tail = (f: string) => {
      try {
        return readFileSync(f, 'utf8').split('\n').filter((l) => !l.startsWith('[mbot-')).slice(-40).join('\n')
      } catch {
        return ''
      }
    }
    return `----- server -----\n${tail(this.serverLog)}\n----- sim -----\n${tail(this.simLog)}`
  }

  /** Lines of the server log that contain text. */
  serverLines(text: string): string[] {
    return readFileSync(this.serverLog, 'utf8').split('\n').filter((l) => l.includes(text))
  }

  private async call(method: string, path: string, body: unknown, headers: Record<string, string>, expect: number[]) {
    const resp = await fetch(this.base + path, {
      method,
      headers: { 'Content-Type': 'application/json', ...headers },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
    const raw = await resp.text()
    if (!expect.includes(resp.status)) throw new Error(`${method} ${path}: HTTP ${resp.status} ${raw}\n${this.logs()}`)
    const cookie = resp.headers.get('set-cookie')
    if (cookie) this.cookie = cookie.split(';')[0]
    return raw ? JSON.parse(raw) : null
  }

  adminCall(method: string, path: string, body?: unknown, expect = [200, 201, 204]) {
    return this.call(method, path, body, this.cookie ? { Cookie: this.cookie } : {}, expect)
  }

  teamCall(team: Team, method: string, path: string, body?: unknown, expect = [200, 201, 202, 204]) {
    return this.call(method, `/t/${team.token}${path}`, body, {}, expect)
  }

  async robotOnline(robot: string) {
    await this.until(`heartbeat of ${robot}`, async () => {
      const st = await this.adminCall('GET', '/admin/api/state')
      return st.robots.some((r: any) => r.id === robot && r.online && r.accepted && r.state.seq > 0)
    })
  }

  /** A team joins, gets an open task and accepts it, and gets the robots. */
  async team(name: string, robots: string[], task = 'Werkzeuge ausprobieren'): Promise<Team> {
    const team: Team = await this.call('POST', '/api/join', { code: this.labCode, name }, {}, [201])
    const t = await this.adminCall('POST', '/admin/api/tasks', { title: task, body: 'Probiert die Werkzeuge aus.' })
    await this.adminCall('POST', `/admin/api/tasks/${t.id}/publish`)
    await this.adminCall('POST', `/admin/api/tasks/${t.id}/assign`, { teams: [team.id] })
    await this.teamCall(team, 'POST', `/tasks/${t.id}/accept`, { prompt: 'empty' })
    for (const r of robots) await this.adminCall('POST', '/admin/api/assign', { robot: r, team: team.id })
    return team
  }

  page(team: Team) {
    return `${this.base}/t/${team.token}`
  }
}
