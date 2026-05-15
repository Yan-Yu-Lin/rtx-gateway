export default defineEventHandler((event) => {
  return { authenticated: Boolean(getAdminSession(event)) }
})
