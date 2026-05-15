import { createError, readBody } from 'h3'

export default defineEventHandler(async (event) => {
  const body = await readBody<{ password?: string }>(event)
  if (!body.password || !validLoginPassword(event, body.password)) {
    throw createError({
      statusCode: 401,
      statusMessage: 'Invalid passphrase',
    })
  }

  setAdminSession(event)
  return { ok: true }
})
