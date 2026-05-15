import type { KeysResponse } from '../../../types/admin'

export default defineEventHandler((event) => {
  return adminApiFetch<KeysResponse>(event, '/admin/v1/keys')
})
