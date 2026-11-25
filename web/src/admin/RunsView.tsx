// "Läufe" of a task: every team with its runs and the four measures, the
// ranking, and the lab lead's buttons. Loaded on demand.
import { useEffect, useRef, useState } from 'preact/hooks'
import { interventionCount, interventionText, running, successText, timeText, type RunView, type TeamScore } from './runs'

export interface RunsViewProps {
  taskId: string
  title: string
  /** Changes when the server state changes (scores, agents): reload. */
  version: string
  call: (method: string, path: string, body?: unknown) => Promise<any>
  act: (p: Promise<unknown>, ok?: string) => Promise<void>
  onClose: () => void
}

function RunRow({ r, p, reload }: { r: RunView; p: RunsViewProps; reload: () => void }) {
  const confirm = (success: boolean) =>
    void p
      .act(p.call('POST', `/runs/${r.run}/confirm`, { success }), success ? 'Als geschafft gewertet' : 'Als nicht geschafft gewertet')
      .then(reload)
  return (
    <tr data-success={r.success === null ? 'open' : String(r.success)}>
      <td>
        Lauf {r.run}
        <span class="runs-sub">Version {r.version}</span>
      </td>
      <td>{running(r) ? 'läuft' : r.end_reason === 'finish' ? 'finish' : r.end_reason}</td>
      <td class="runs-success">{successText(r)}</td>
      <td>{timeText(r.to_goal_s)}</td>
      <td>{r.calls}</td>
      <td title={interventionText(r.interventions)}>
        {interventionCount(r.interventions)}
        <span class="runs-sub">{interventionText(r.interventions)}</span>
      </td>
      <td>
        <details>
          <summary>{r.team_tools.length ? r.team_tools.join(', ') : 'nur eingebaute'}</summary>
          <p class="runs-sub">Eingebaut: {r.tools.join(', ')}</p>
          {r.report && <q class="runs-report">{r.report}</q>}
        </details>
      </td>
      <td>
        <div class="runs-actions">
          <button class="btn btn-small" disabled={running(r)} onClick={() => confirm(true)} aria-label={`Lauf ${r.run} geschafft`}>
            ✓ geschafft
          </button>
          <button class="btn btn-small btn-quiet" disabled={running(r)} onClick={() => confirm(false)} aria-label={`Lauf ${r.run} nicht geschafft`}>
            ✗ nicht
          </button>
          <button
            class="btn-link"
            onClick={() =>
              void p
                .act(
                  p.call('POST', `/runs/${r.run}/intervention`, {
                    kind: 'moved',
                  }),
                  'Eingriff notiert',
                )
                .then(reload)
            }
          >
            + von Hand bewegt
          </button>
        </div>
      </td>
    </tr>
  )
}

export default function RunsView(p: RunsViewProps) {
  const dlg = useRef<HTMLDialogElement>(null)
  const [teams, setTeams] = useState<TeamScore[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const reload = () =>
    p.call('GET', `/tasks/${p.taskId}/runs`).then(
      (d) => setTeams(d.teams ?? []),
      (e) => setError((e as Error).message),
    )
  useEffect(() => {
    dlg.current?.showModal()
  }, [])
  useEffect(() => {
    void reload()
  }, [p.version])

  return (
    <dialog ref={dlg} class="task-dialog runs-dialog" aria-labelledby="runs-title" onCancel={(e) => (e.preventDefault(), p.onClose())}>
      <header class="task-dialog-head">
        <h2 id="runs-title">Läufe: {p.title}</h2>
        <span class="spacer" />
        <button class="btn btn-quiet btn-small" onClick={p.onClose}>
          Schließen
        </button>
      </header>
      <div class="runs-body">
        {error && <p class="error-text">{error}</p>}
        {teams === null ? (
          <div class="loading" aria-busy="true" />
        ) : teams.length === 0 ? (
          <p class="pool-sub">Noch keine Läufe in dieser Aufgabe.</p>
        ) : (
          <>
            <section>
              <h3>Rangliste</h3>
              <p class="pool-sub">
                Zuerst die geschafften Läufe, dann die wenigsten Modellaufrufe, dann die kürzeste Zeit. Gewertet wird der beste Lauf jedes Teams.
              </p>
              <table class="runs-table runs-ranking">
                <thead>
                  <tr>
                    <th>Rang</th>
                    <th>Team</th>
                    <th>Geschafft</th>
                    <th>Modellaufrufe</th>
                    <th>Zeit bis zum Ziel</th>
                    <th>Eingriffe</th>
                  </tr>
                </thead>
                <tbody>
                  {teams.map((t) => (
                    <tr key={t.team_id}>
                      <td>{t.rank}.</td>
                      <td>{t.name}</td>
                      <td>{t.successes}</td>
                      <td>{t.best?.calls ?? '–'}</td>
                      <td>{timeText(t.best?.to_goal_s)}</td>
                      <td>{t.best ? interventionCount(t.best.interventions) : '–'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </section>
            {teams.map((t) => (
              <section key={t.team_id} class="runs-team">
                <div class="runs-team-head">
                  <h3>{t.name}</h3>
                  <button
                    class="btn btn-small"
                    onClick={() => void p.act(p.call('POST', `/teams/${t.team_id}/start`), `Lauf von ${t.name} gestartet`).then(reload)}
                  >
                    Lauf starten
                  </button>
                </div>
                <table class="runs-table">
                  <thead>
                    <tr>
                      <th>Lauf</th>
                      <th>Ende</th>
                      <th>Erfolg</th>
                      <th>Zeit</th>
                      <th>Modellaufrufe</th>
                      <th>Eingriffe</th>
                      <th>Werkzeuge</th>
                      <th />
                    </tr>
                  </thead>
                  <tbody>
                    {t.runs.map((r) => (
                      <RunRow key={r.run} r={r} p={p} reload={reload} />
                    ))}
                  </tbody>
                </table>
              </section>
            ))}
          </>
        )}
      </div>
    </dialog>
  )
}
