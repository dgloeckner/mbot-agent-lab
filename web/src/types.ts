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
}

export interface LogEntry {
  id: number
  at: string
  source: string // robot | team | system | agent:<name>
  kind: string
  robot?: string
  text?: string
  data?: Record<string, unknown>
}

export type AgentState = 'waiting' | 'thinking' | 'acting'

export interface Session {
  team: Team
  robots: Robot[]
  agent: AgentState
  agent_available: boolean
  prompt: PromptVersion | null
  versions: PromptVersion[]
  notes: Note[]
  tools: ToolInfo[]
  limits: { prompt_chars: number; message_chars: number; note_chars: number }
}
