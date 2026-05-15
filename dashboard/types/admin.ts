export interface ApiKey {
  id: string
  name: string
  prefix: string
  key?: string
  scopes: string[]
  enabled: boolean
  created_at: string
  updated_at: string
  last_used_at?: string
  revoked_at?: string
}

export interface KeysResponse {
  keys: ApiKey[]
}

export interface UsageSummaryRow {
  bucket: string
  requests: number
  errors: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
}

export interface UsageSummaryResponse {
  from: string
  to: string
  group_by: 'day'
  rows: UsageSummaryRow[]
}

export interface UsageRequest {
  request_id: string
  api_key_id?: string
  api_key_prefix?: string
  endpoint_id: string
  host: string
  method: string
  path: string
  model?: string
  streaming: boolean
  usage_missing: boolean
  prompt_tokens?: number
  completion_tokens?: number
  total_tokens?: number
  status_code: number
  upstream_status_code?: number
  latency_ms: number
  client_ip?: string
  user_agent?: string
  error?: string
  created_at: string
}

export interface UsageRequestsResponse {
  requests: UsageRequest[]
}

export interface EndpointHealth {
  id: string
  host: string
  upstream_url: string
  enabled: boolean
  last_health_status?: string
  last_health_status_code?: number
  last_health_latency_ms?: number
  last_health_error?: string
  last_health_checked_at?: string
}

export interface HealthResponse {
  endpoints: EndpointHealth[]
}

export interface HealthCheckResult {
  endpoint_id: string
  status: string
  status_code?: number
  latency_ms: number
  error?: string
  checked_at: string
}

export interface HealthCheckResponse {
  results: HealthCheckResult[]
}

export interface HealthCheckLog {
  id: number
  endpoint_id: string
  status: string
  status_code?: number
  latency_ms: number
  error?: string
  checked_at: string
}

export interface HealthChecksResponse {
  checks: HealthCheckLog[]
}
