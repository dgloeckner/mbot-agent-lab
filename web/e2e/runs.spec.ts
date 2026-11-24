// The control room's "Läufe": two teams finish a run each with their own
// team tool, the lab lead confirms one and rejects the other, and the ranking
// puts the confirmed team first.

import { expect, test } from "@playwright/test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { Lab, ROOT, SHOTS, type Team } from "./lab";

let lab: Lab;

test.beforeAll(async () => {
  lab = await Lab.start({ robots: 2 });
});

test.afterAll(async () => {
  await lab?.stop();
});

const quadrat = JSON.parse(
  readFileSync(
    join(ROOT, "docs", "examples", "programs", "quadrat.json"),
    "utf-8",
  ),
);

async function finishedRun(team: Team, task: string) {
  await lab.adminCall("POST", `/admin/api/tasks/${task}/assign`, {
    teams: [team.id],
  });
  await lab.teamCall(team, "POST", `/tasks/${task}/accept`, {
    prompt: "empty",
  });
  await lab.teamCall(team, "POST", "/programs", {
    name: "quadrat",
    description: "Fährt ein Quadrat.",
    blockly: quadrat.blockly,
  });
  await lab.teamCall(team, "POST", "/prompt", {
    text: "Fahre mit `quadrat` ein Quadrat.",
  });
  await new Promise((r) => setTimeout(r, 2500)); // the robot learns its owner from the next heartbeat
  await lab.teamCall(team, "POST", "/start");
}

test("the lab lead scores the runs of a task", async ({ page }) => {
  test.setTimeout(120_000);
  const task = await lab.adminCall("POST", "/admin/api/tasks", {
    title: "Challenge: Quadrat",
    body: "Fahrt ein Quadrat.",
    agent_tools: "team_tools",
  });
  await lab.adminCall("POST", `/admin/api/tasks/${task.id}/publish`);
  const blau = await lab.team("Blau", ["mbot-1"]);
  const gelb = await lab.team("Gelb", ["mbot-2"]);
  await Promise.all([finishedRun(blau, task.id), finishedRun(gelb, task.id)]);

  const runsOf = async () =>
    (await lab.adminCall("GET", `/admin/api/tasks/${task.id}/runs`))
      .teams as any[];
  await lab.until(
    "two finished runs",
    async () => {
      const teams = await runsOf();
      return (
        teams.length === 2 &&
        teams.every((t) => t.runs.length > 0 && t.runs[0].ended_at)
      );
    },
    60_000,
  );

  await page.goto(lab.base + "/admin");
  await page.locator('input[type="password"]').fill(lab.adminPassword);
  await page.getByRole("button", { name: "Anmelden" }).click();
  await page
    .getByRole("button", { name: "Läufe von Challenge: Quadrat" })
    .click();
  const dlg = page.locator(".runs-dialog");
  await expect(dlg.locator(".runs-ranking tbody tr")).toHaveCount(2);

  // By heading: a robot may be called "blau" too.
  const section = (name: string) =>
    dlg
      .locator(".runs-team")
      .filter({ has: page.getByRole("heading", { name, exact: true }) });
  const blauRuns = section("Blau");
  const gelbRuns = section("Gelb");
  await expect(blauRuns.locator(".runs-success")).toHaveText("offen");
  await gelbRuns.getByRole("button", { name: /nicht geschafft/ }).click();
  await expect(gelbRuns.locator(".runs-success")).toHaveText("nicht geschafft");
  await blauRuns
    .getByRole("button", { name: /geschafft$/ })
    .first()
    .click();
  await expect(blauRuns.locator(".runs-success")).toHaveText("geschafft");
  await blauRuns.getByRole("button", { name: "+ von Hand bewegt" }).click();
  await expect(blauRuns.locator("td", { hasText: "1 bewegt" })).toBeVisible();
  await expect(blauRuns.locator("summary")).toHaveText("quadrat");

  const first = dlg.locator(".runs-ranking tbody tr").first();
  await expect(first).toContainText("1.");
  await expect(first.locator("td").nth(1)).toHaveText("Blau");
  await expect(
    dlg.locator(".runs-ranking tbody tr").nth(1).locator("td").nth(1),
  ).toHaveText("Gelb");
  await dlg.screenshot({ path: join(SHOTS, "runs-view.png") });

  // Another run from the control room: it shows up as running.
  await gelbRuns.getByRole("button", { name: "Lauf starten" }).click();
  await expect(gelbRuns.locator("tbody tr")).toHaveCount(2);
});
