import type { SecurityEventsResponse } from '../../../../types/admin'

export default defineEventHandler((event) => {
  return adminApiFetch<SecurityEventsResponse>(event, adminPathWithQuery(event, '/admin/v1/security/events'))
})
