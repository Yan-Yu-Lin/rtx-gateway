<template>
  <div>
    <PageHeader
      eyebrow="Access"
      title="API keys"
      description="Create scoped keys for LLM and OCR clients."
    />

    <section v-if="createdKey" class="mb-6 rounded-lg border border-cyan-500/30 bg-cyan-950/30 p-4">
      <div class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div>
          <h3 class="font-semibold text-cyan-100">New key created</h3>
          <p class="mt-1 break-all font-mono text-sm text-cyan-50">{{ createdKey.key }}</p>
        </div>
        <button class="button-secondary shrink-0" @click="copyKey">Copy</button>
      </div>
    </section>

    <section class="panel p-4">
      <h3 class="text-lg font-semibold text-zinc-50">Create key</h3>
      <form class="mt-4 grid gap-4 lg:grid-cols-[1fr_auto_auto]" @submit.prevent="createKey">
        <label>
          <span class="text-sm font-medium text-zinc-300">Name</span>
          <input v-model="form.name" class="field mt-2" placeholder="PII platform dev">
        </label>
        <fieldset>
          <legend class="text-sm font-medium text-zinc-300">Scopes</legend>
          <div class="mt-2 flex gap-2">
            <label
              v-for="scope in scopeOptions"
              :key="scope"
              class="flex cursor-pointer items-center gap-2 rounded-md border border-zinc-700 px-3 py-2 text-sm text-zinc-200"
            >
              <input v-model="form.scopes" type="checkbox" :value="scope" class="accent-cyan-400">
              {{ scope }}
            </label>
          </div>
        </fieldset>
        <div class="flex items-end">
          <button class="button-primary w-full" :disabled="creating || !form.name || form.scopes.length === 0">
            {{ creating ? 'Creating...' : 'Create' }}
          </button>
        </div>
      </form>
      <p v-if="formError" class="mt-3 rounded-md bg-red-950/70 p-3 text-sm text-red-200">{{ formError }}</p>
    </section>

    <section class="panel mt-6 p-4">
      <div class="mb-4 flex items-center justify-between">
        <h3 class="text-lg font-semibold text-zinc-50">Keys</h3>
        <button class="button-secondary" @click="refresh">Refresh</button>
      </div>
      <div class="overflow-x-auto">
        <table class="min-w-full divide-y divide-zinc-800">
          <thead>
            <tr>
              <th class="table-heading">Name</th>
              <th class="table-heading">Prefix</th>
              <th class="table-heading">Scopes</th>
              <th class="table-heading">Last used</th>
              <th class="table-heading">Status</th>
              <th class="table-heading"></th>
            </tr>
          </thead>
          <tbody class="divide-y divide-zinc-800">
            <tr v-for="key in keys" :key="key.id">
              <td class="table-cell font-medium">{{ key.name }}</td>
              <td class="table-cell font-mono text-xs">{{ key.prefix }}</td>
              <td class="table-cell">{{ key.scopes.join(', ') }}</td>
              <td class="table-cell whitespace-nowrap text-zinc-400">{{ formatDateTime(key.last_used_at) }}</td>
              <td class="table-cell">
                <span
                  class="rounded-full px-2 py-1 text-xs font-semibold"
                  :class="key.enabled ? 'bg-emerald-500/15 text-emerald-300' : 'bg-zinc-700 text-zinc-300'"
                >
                  {{ key.enabled ? 'active' : 'revoked' }}
                </span>
              </td>
              <td class="table-cell text-right">
                <button class="button-danger" :disabled="!key.enabled || revoking === key.id" @click="revokeKey(key.id)">
                  Revoke
                </button>
              </td>
            </tr>
            <tr v-if="keys.length === 0">
              <td class="table-cell text-zinc-500" colspan="6">No keys created yet.</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import type { ApiKey, KeysResponse } from '~/types/admin'

const scopeOptions = ['llm', 'ocr']
const form = reactive({
  name: '',
  scopes: ['llm', 'ocr'],
})
const creating = ref(false)
const revoking = ref('')
const formError = ref('')
const createdKey = ref<ApiKey | null>(null)

const { data, refresh } = await useFetch<KeysResponse>('/api/admin/keys')
const keys = computed(() => data.value?.keys || [])

async function createKey() {
  creating.value = true
  formError.value = ''
  try {
    createdKey.value = await $fetch<ApiKey>('/api/admin/keys', {
      method: 'POST',
      body: {
        name: form.name,
        scopes: form.scopes,
      },
    })
    form.name = ''
    await refresh()
  } catch {
    formError.value = 'Could not create key.'
  } finally {
    creating.value = false
  }
}

async function revokeKey(id: string) {
  if (!confirm('Revoke this API key?')) return
  revoking.value = id
  try {
    await $fetch(`/api/admin/keys/${encodeURIComponent(id)}/revoke`, { method: 'POST' })
    await refresh()
  } finally {
    revoking.value = ''
  }
}

async function copyKey() {
  if (!createdKey.value?.key) return
  await navigator.clipboard.writeText(createdKey.value.key)
}
</script>
