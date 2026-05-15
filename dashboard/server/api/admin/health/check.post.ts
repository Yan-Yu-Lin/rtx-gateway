import type { HealthCheckResponse } from '../../../../types/admin'

export default defineEventHandler((event) => {
  return adminApiFetch<HealthCheckResponse>(event, '/admin/v1/health/check', {
    method: 'POST',
  })
})
