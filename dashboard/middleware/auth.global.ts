export default defineNuxtRouteMiddleware(async (to) => {
  if (to.path === '/login') return

  const headers = import.meta.server ? useRequestHeaders(['cookie']) : undefined
  const session = await $fetch<{ authenticated: boolean }>('/api/auth/session', { headers }).catch(() => ({ authenticated: false }))
  if (!session.authenticated) {
    return navigateTo('/login')
  }
})
