import { createHmac, timingSafeEqual } from 'node:crypto'
import {
  createError,
  deleteCookie,
  getCookie,
  setCookie,
  type H3Event,
} from 'h3'

const sessionCookieName = 'rtx_gateway_session'
const sessionMaxAgeSeconds = 12 * 60 * 60

interface SessionPayload {
  sub: 'arthur'
  iat: number
  exp: number
}

export function setAdminSession(event: H3Event): void {
  const now = Math.floor(Date.now() / 1000)
  const payload: SessionPayload = {
    sub: 'arthur',
    iat: now,
    exp: now + sessionMaxAgeSeconds,
  }

  setCookie(event, sessionCookieName, signPayload(event, payload), {
    httpOnly: true,
    sameSite: 'lax',
    secure: process.env.NODE_ENV === 'production',
    path: '/',
    maxAge: sessionMaxAgeSeconds,
  })
}

export function clearAdminSession(event: H3Event): void {
  deleteCookie(event, sessionCookieName, { path: '/' })
}

export function getAdminSession(event: H3Event): SessionPayload | null {
  const raw = getCookie(event, sessionCookieName)
  if (!raw) return null

  const parts = raw.split('.')
  if (parts.length !== 2) return null

  const [encodedPayload, signature] = parts
  if (!encodedPayload || !signature) return null

  const expected = sign(encodedPayload, sessionSecret(event))
  if (!constantTimeEqual(signature, expected)) return null

  try {
    const payload = JSON.parse(Buffer.from(encodedPayload, 'base64url').toString('utf8')) as SessionPayload
    if (payload.sub !== 'arthur' || payload.exp < Math.floor(Date.now() / 1000)) {
      return null
    }
    return payload
  } catch {
    return null
  }
}

export function requireAdminSession(event: H3Event): SessionPayload {
  const session = getAdminSession(event)
  if (!session) {
    throw createError({
      statusCode: 401,
      statusMessage: 'Login required',
    })
  }
  return session
}

export function validLoginPassword(event: H3Event, candidate: string): boolean {
  const expected = runtimeString(event, 'loginPassword', 'NUXT_LOGIN_PASSWORD', 1)
  return constantTimeEqual(candidate, expected)
}

function signPayload(event: H3Event, payload: SessionPayload): string {
  const encoded = Buffer.from(JSON.stringify(payload)).toString('base64url')
  return `${encoded}.${sign(encoded, sessionSecret(event))}`
}

function sessionSecret(event: H3Event): string {
  return runtimeString(event, 'sessionSecret', 'NUXT_SESSION_SECRET', 16)
}

function runtimeString(event: H3Event, key: 'sessionSecret' | 'loginPassword', envName: string, minLength: number): string {
  const value = String(useRuntimeConfig(event)[key] || '')
  if (value.length < minLength) {
    throw createError({
      statusCode: 500,
      statusMessage: `${envName} is not configured`,
    })
  }
  return value
}

function sign(value: string, secret: string): string {
  return createHmac('sha256', secret).update(value).digest('base64url')
}

function constantTimeEqual(actual: string, expected: string): boolean {
  const actualBuffer = Buffer.from(actual)
  const expectedBuffer = Buffer.from(expected)
  if (actualBuffer.length !== expectedBuffer.length) return false
  return timingSafeEqual(actualBuffer, expectedBuffer)
}
