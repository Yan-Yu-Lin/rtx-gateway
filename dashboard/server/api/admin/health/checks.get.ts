import type { HealthChecksResponse } from '../../../../types/admin'

export default defineEventHandler((event) => {
  return adminApiFetch<HealthChecksResponse>(event, adminPathWithQuery(event, '/admin/v1/health/checks'))
})
