<template>
  <main class="flex min-h-screen items-center justify-center bg-zinc-950 px-4">
    <section class="w-full max-w-sm rounded-lg border border-zinc-800 bg-zinc-900 p-6">
      <p class="text-xs font-semibold uppercase tracking-[0.24em] text-cyan-400">RTX Gateway</p>
      <h1 class="mt-2 text-2xl font-semibold text-zinc-50">Sign in</h1>

      <form class="mt-6 space-y-4" @submit.prevent="login">
        <label class="block">
          <span class="text-sm font-medium text-zinc-300">Passphrase</span>
          <input
            v-model="password"
            class="field mt-2"
            type="password"
            autocomplete="current-password"
            autofocus
          >
        </label>

        <p v-if="errorMessage" class="rounded-md bg-red-950/70 p-3 text-sm text-red-200">
          {{ errorMessage }}
        </p>

        <button class="button-primary w-full" :disabled="pending || !password">
          {{ pending ? 'Signing in...' : 'Sign in' }}
        </button>
      </form>
    </section>
  </main>
</template>

<script setup lang="ts">
definePageMeta({ layout: 'bare' })

const password = ref('')
const pending = ref(false)
const errorMessage = ref('')

async function login() {
  pending.value = true
  errorMessage.value = ''
  try {
    await $fetch('/api/auth/login', {
      method: 'POST',
      body: { password: password.value },
    })
    await navigateTo('/')
  } catch {
    errorMessage.value = 'Invalid passphrase.'
  } finally {
    pending.value = false
  }
}
</script>
