export default defineNuxtConfig({
  compatibilityDate: '2026-05-15',
  modules: ['@nuxtjs/tailwindcss'],
  css: ['~/assets/css/main.css'],
  devtools: { enabled: false },
  runtimeConfig: {
    adminApiUrl: process.env.NUXT_ADMIN_API_URL || 'http://127.0.0.1:9189',
    adminToken: process.env.NUXT_ADMIN_TOKEN || '',
    sessionSecret: process.env.NUXT_SESSION_SECRET || '',
    loginPassword: process.env.NUXT_LOGIN_PASSWORD || '',
  },
  typescript: {
    strict: true,
  },
})
