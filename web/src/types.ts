// Shapes of the team API (server/internal/web/team.go).

export interface Team {
  id: string
  name: string
  color: string
}

export interface RobotState {
  skill: string
  distance_cm: number
  color: string
  line: string
  yaw: number
  battery: number
  tilted: boolean
  seq: number
  t_ms: number
}

export interface Robot {
  alias: string
  id: string
  color: string
  online: boolean
  accepted: boolean
  state: RobotState
}

export interface PromptVersion {
  version: number
  text?: string
  created_at: string
  chars: number
  /** Team tools that are new, changed or removed in this version (the version list only). */
  tools?: ToolChange[]
}

export interface ToolChange {
  id: string
  name: string
  change: 'neu' | 'geändert' | 'entfernt'
}

/** One difference of a team tool between two states (server: programs.Change). */
export interface BlockChange {
  kind: 'value' | 'new' | 'removed' | 'moved' | 'name' | 'description'
  block?: string
  field?: string
  old?: unknown
  new?: unknown
}

/** Team tools that differ between two states; version 0 is the working state. */
export interface ToolChanges {
  left: number
  right: number
  tools: ToolChange[]
}

/** A team tool as stored in a version or the working state. */
export interface ToolState {
  id: string
  name: string
  description: string
  blockly: unknown
}

/** A team tool in two states, the older on the left; a side's tool is null where it did not exist. */
export interface ToolDiff {
  left: { version: number; tool: ToolState | null }
  right: { version: number; tool: ToolState | null }
  changes: BlockChange[]
}

/** A team tool of a deployed version and whether the agent has it. */
export interface VersionTool {
  id: string
  name: string
  agent: boolean
  why?: string
}

export interface Note {
  id: number
  text: string
  author: 'agent' | 'team'
  created_at: string
}

export interface ToolInfo {
  name: string
  group: string
  params: string
  summary: string
  ready: boolean
  /** A person can run it by hand in the console. */
  manual: boolean
  /** JSON schema of the arguments, the one the model sees; only for by-hand tools. */
  schema?: JSONSchema
}

/** The subset of JSON Schema the tool registry uses. */
export interface JSONSchema {
  type?: string
  properties?: Record<string, JSONSchema>
  required?: string[]
  enum?: string[]
  minimum?: number
  maximum?: number
  maxLength?: number
  description?: string
}

export interface LogEntry {
  id: number
  at: string
  source: string // robot | team | system | agent:<name>
  kind: string
  robot?: string
  text?: string
  data?: Record<string, unknown>
  /** 'conversation' for the talk with the agent while no run is active; unset for the run protocol. */
  channel?: 'conversation'
  task_id?: string
}

export type AgentState = 'waiting' | 'thinking' | 'acting'

export interface AgentStatus {
  state: AgentState
  running: boolean
  run_id?: number
  task_id?: string
  provider?: string
  model?: string
  started_at?: string
  tokens_in: number
  tokens_out: number
  cost_micro_eur: number
  calls: number
  /** The team can talk with its agent while no run is active. */
  conversation: boolean
  conversation_id?: number
  conversation_tokens?: number
}

/** Sum of all runs and conversations of a team; `conversation` is the conversations' share. */
export interface Totals {
  runs: number
  tokens_in: number
  tokens_out: number
  cost_micro_eur: number
  conversation: { count: number; tokens_in: number; tokens_out: number; cost_micro_eur: number }
}

/** A language model the team may choose (server/internal/lab/models.go). */
export interface ModelOption {
  provider: string
  model: string
  price_in: number // per million input tokens
  price_out: number // per million output tokens
  default: boolean
  allowed: boolean
}

export interface TeamModels {
  selected: string // provider of the team's next run
  options: ModelOption[]
}

export type TaskState = 'draft' | 'open' | 'closed'
export type TeamTaskStatus = 'assigned' | 'accepted' | 'active'

/** A task assigned to the team (server/internal/store/tasks.go TeamTaskInfo). Lists leave out body and starter. */
export interface TeamTask {
  id: string
  title: string
  body: string
  starter: string
  state: TaskState
  status: TeamTaskStatus
  assigned_at: string
  accepted_at?: string
  /** all: the agent has every tool; team_tools: only the team's own and a few that move no robot. */
  agent_tools?: 'all' | 'team_tools'
}

export type PromptChoice = 'starter' | 'empty' | 'copy'

export interface Session {
  team: Team
  /** The active task with its text; null while the team waits for one. */
  task: TeamTask | null
  tasks: TeamTask[]
  robots: Robot[]
  agent: AgentStatus
  total: Totals
  agent_available: boolean
  models: TeamModels
  prompt: PromptVersion | null
  versions: PromptVersion[]
  notes: Note[]
  tools: ToolInfo[]
  /** The built-in tools the agent keeps in a task with agent_tools team_tools. */
  team_only_tools?: string[]
  limits: { prompt_chars: number; message_chars: number; note_chars: number }
}

/** A problem of a team tool, at its block (empty: name or description). */
export interface ProgramProblem {
  block?: string
  /** The input the problem is about, when it is not a block. */
  field?: 'name' | 'description'
  text: string
}

/** A team tool ("eigenes Werkzeug") in the list. */
export interface ProgramInfo {
  id: string
  name: string
  description: string
  valid: boolean
  problems: ProgramProblem[]
  updated_at: string
  /** Against the newest version: as deployed, changed since, or not deployed yet. */
  deploy?: 'deployed' | 'changed' | 'new'
  /** Whether the agent has the tool in the newest version, and if not, why. */
  agent: boolean
  why?: string
}

/** A team tool with its Blockly state. */
export interface ProgramDetail extends ProgramInfo {
  blockly: unknown
}

/** How a team tool ended (server: programs.Result). */
export interface ProgramResult {
  ok: boolean
  end: 'done' | 'aborted' | 'limit' | 'stopped'
  steps: number
  runtime_s: number
  at?: string
  block?: string
  tool?: string
  reason?: string
  error?: string
  last?: Record<string, any>
  value?: unknown
}

/** An SSE "program" event: a step of a running team tool. */
export interface ProgramEvent {
  run: string
  program: string
  phase: 'start' | 'wait' | 'before' | 'after' | 'end'
  name?: string
  robot?: string
  step?: number | boolean
  block?: string
  kind?: 'call' | 'sense'
  tool?: string
  args?: Record<string, unknown>
  round?: number
  result?: any
  ms?: number
}
