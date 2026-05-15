import { createError, getRequestURL, type H3Event } from 'h3'

export async function adminApiFetch<T>(event: H3Event, path: string, options: Parameters<typeof $fetch<T>>[1] = {}): Promise<T> {
  requireAdminSession(event)

  const config = useRuntimeConfig(event)
  const adminApiUrl = String(config.adminApiUrl || '').replace(/\/$/, '')
  const adminToken = String(config.adminToken || '')
  if (!adminApiUrl || !adminToken) {
    throw createError({
      statusCode: 500,
      statusMessage: 'Admin API URL or token is not configured',
    })
  }

  return await $fetch<T>(`${adminApiUrl}${path}`, {
    ...options,
    headers: {
      ...(options.headers || {}),
      Authorization: `Bearer ${adminToken}`,
    },
  })
}

export function adminPathWithQuery(event: H3Event, path: string): string {
  return `${path}${getRequestURL(event).search}`
}
