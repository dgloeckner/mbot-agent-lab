import { describe, expect, it } from "vitest";
import {
  interventionCount,
  interventionText,
  successText,
  timeText,
  type RunView,
} from "./runs";

const run = (p: Partial<RunView>): RunView =>
  ({
    run: 1,
    team_id: "t",
    version: 1,
    model: "m",
    started_at: "",
    calls: 3,
    success: null,
    interventions: { stops: 0, by_hand: 0, moved: 0 },
    tools: [],
    team_tools: [],
    ...p,
  }) as RunView;

describe("runs view", () => {
  it("words success, time and interventions", () => {
    expect(successText(run({}))).toBe("offen");
    expect(successText(run({ success: true }))).toBe("geschafft");
    expect(successText(run({ success: false }))).toBe("nicht geschafft");
    expect(timeText(42.5)).toBe("42,5 s");
    expect(timeText(undefined)).toBe("–");
    const i = { stops: 1, by_hand: 2, moved: 1 };
    expect(interventionText(i)).toBe("2 von Hand, 1 Stopp, 1 bewegt");
    expect(interventionCount(i)).toBe(4);
    expect(interventionText({ stops: 0, by_hand: 0, moved: 0 })).toBe("keine");
  });
});
