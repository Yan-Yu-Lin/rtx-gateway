<template>
  <div class="min-h-screen">
    <aside class="fixed inset-y-0 left-0 hidden w-64 border-r border-zinc-800 bg-zinc-950 px-5 py-6 lg:block">
      <NuxtLink to="/" class="block">
        <p class="text-xs font-semibold uppercase tracking-[0.24em] text-cyan-400">RTX Gateway</p>
        <h1 class="mt-2 text-xl font-semibold text-zinc-50">Control plane</h1>
      </NuxtLink>

      <nav class="mt-8 space-y-1">
        <NuxtLink
          v-for="item in nav"
          :key="item.to"
          :to="item.to"
          class="block rounded-md px-3 py-2 text-sm font-medium transition"
          :class="route.path === item.to ? 'bg-zinc-800 text-zinc-50' : 'text-zinc-400 hover:bg-zinc-900 hover:text-zinc-100'"
        >
          {{ item.label }}
        </NuxtLink>
      </nav>

      <button class="button-secondary mt-8 w-full" @click="logout">
        Log out
      </button>
    </aside>

    <div class="lg:pl-64">
      <header class="sticky top-0 z-10 border-b border-zinc-800 bg-zinc-950/90 px-4 py-3 backdrop-blur lg:hidden">
        <div class="flex items-center justify-between">
          <NuxtLink to="/" class="font-semibold text-zinc-50">RTX Gateway</NuxtLink>
          <button class="button-secondary px-2 py-1" @click="logout">Log out</button>
        </div>
        <nav class="mt-3 flex gap-2 overflow-x-auto pb-1">
          <NuxtLink
            v-for="item in nav"
            :key="item.to"
            :to="item.to"
            class="rounded-md px-3 py-1.5 text-sm font-medium"
            :class="route.path === item.to ? 'bg-zinc-800 text-zinc-50' : 'text-zinc-400'"
          >
            {{ item.label }}
          </NuxtLink>
        </nav>
      </header>

      <main class="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
const route = useRoute()
const nav = [
  { to: '/', label: 'Overview' },
  { to: '/keys', label: 'Keys' },
  { to: '/usage', label: 'Usage' },
  { to: '/health', label: 'Health' },
]

async function logout() {
  await $fetch('/api/auth/logout', { method: 'POST' }).catch(() => null)
  await navigateTo('/login')
}
</script>
