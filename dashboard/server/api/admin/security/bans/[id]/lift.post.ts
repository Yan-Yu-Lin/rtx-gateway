import { getRouterParam } from 'h3'

export default defineEventHandler((event) => {
  const id = getRouterParam(event, 'id')
  return adminApiFetch<{ ok: boolean }>(event, `/admin/v1/security/bans/${encodeURIComponent(id || '')}/lift`, {
    method: 'POST',
  })
})
