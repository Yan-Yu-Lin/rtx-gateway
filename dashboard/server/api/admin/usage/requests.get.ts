import type { UsageRequestsResponse } from '../../../../types/admin'

export default defineEventHandler((event) => {
  return adminApiFetch<UsageRequestsResponse>(event, adminPathWithQuery(event, '/admin/v1/usage/requests'))
})
