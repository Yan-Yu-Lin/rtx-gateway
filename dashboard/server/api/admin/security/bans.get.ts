import type { SecurityBansResponse } from '../../../../types/admin'

export default defineEventHandler((event) => {
  return adminApiFetch<SecurityBansResponse>(event, '/admin/v1/security/bans')
})
