import { getRouterParam } from 'h3'
import type { ApiKey } from '../../../../../types/admin'

export default defineEventHandler((event) => {
  const id = getRouterParam(event, 'id')
  return adminApiFetch<ApiKey>(event, `/admin/v1/keys/${encodeURIComponent(id || '')}/revoke`, {
    method: 'POST',
  })
})
