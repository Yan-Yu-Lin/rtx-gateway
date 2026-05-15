import type { HealthResponse } from '../../../types/admin'

export default defineEventHandler((event) => {
  return adminApiFetch<HealthResponse>(event, '/admin/v1/health')
})
