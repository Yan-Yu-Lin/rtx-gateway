import type { UsageSummaryResponse } from '../../../../types/admin'

export default defineEventHandler((event) => {
  return adminApiFetch<UsageSummaryResponse>(event, adminPathWithQuery(event, '/admin/v1/usage/summary'))
})
