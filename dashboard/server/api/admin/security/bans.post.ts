import { readBody } from 'h3'
import type { SecurityBan } from '../../../../types/admin'

export default defineEventHandler(async (event) => {
  const body = await readBody(event)
  return adminApiFetch<SecurityBan>(event, '/admin/v1/security/bans', {
    method: 'POST',
    body,
  })
})
