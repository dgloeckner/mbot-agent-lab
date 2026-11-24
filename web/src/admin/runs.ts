// Pure helpers for the control room's "Läufe": the measures of a run in
// words. The ranking itself comes from the server (lab.Rank).

export interface Interventions {
  stops: number;
  by_hand: number;
  moved: number;
}

export interface RunView {
  run: number;
  team_id: string;
  version: number;
  model: string;
  started_at: string;
  ended_at?: string;
  end_reason?: string;
  report?: string;
  calls: number;
  to_goal_s?: number;
  success: boolean | null;
  interventions: Interventions;
  tools: string[];
  team_tools: string[];
}

export interface TeamScore {
  team_id: string;
  name: string;
  runs: RunView[];
  successes: number;
  best?: RunView;
  rank: number;
}

/** "geschafft", "nicht geschafft" or "offen". */
export function successText(r: RunView): string {
  return r.success === null || r.success === undefined
    ? "offen"
    : r.success
      ? "geschafft"
      : "nicht geschafft";
}

/** "42,5 s" or "–" without finish. */
export function timeText(s?: number): string {
  return typeof s === "number"
    ? `${s.toLocaleString("de-DE", { maximumFractionDigits: 1 })} s`
    : "–";
}

/** Interventions in words: "2 von Hand, 1 Stopp, 1 bewegt" or "keine". */
export function interventionText(i: Interventions): string {
  const parts: string[] = [];
  if (i.by_hand) parts.push(`${i.by_hand} von Hand`);
  if (i.stops) parts.push(i.stops === 1 ? "1 Stopp" : `${i.stops} Stopps`);
  if (i.moved) parts.push(`${i.moved} bewegt`);
  return parts.join(", ") || "keine";
}

export const interventionCount = (i: Interventions) =>
  i.by_hand + i.stops + i.moved;

/** Whether a run is still going (no end yet). */
export const running = (r: RunView) => !r.ended_at;
