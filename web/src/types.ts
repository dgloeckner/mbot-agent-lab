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
  limits: { prompt_chars: number; message_chars: number; note_chars: number }
}
