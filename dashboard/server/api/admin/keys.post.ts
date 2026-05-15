import { readBody } from 'h3'
import type { ApiKey } from '../../../types/admin'

export default defineEventHandler(async (event) => {
  const body = await readBody<{ name: string; scopes: string[] }>(event)
  return adminApiFetch<ApiKey>(event, '/admin/v1/keys', {
    method: 'POST',
    body,
  })
})
