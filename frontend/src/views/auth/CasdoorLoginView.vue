<template>
  <main class="flex min-h-screen items-center justify-center bg-gray-50 px-6 dark:bg-dark-900">
    <div class="w-full max-w-sm space-y-4 text-center" aria-live="polite">
      <p v-if="errorKey" role="alert" class="text-sm text-red-600 dark:text-red-400">
        {{ t(errorKey, { providerName: 'IDONE' }) }}
      </p>
      <p v-else role="status" class="text-sm text-gray-600 dark:text-dark-300">
        {{ t('auth.oidc.redirecting', { providerName: 'IDONE' }) }}
      </p>
      <button v-if="errorKey" class="btn btn-primary" :disabled="isLoading" @click="startLogin(true)">
        {{ t('auth.oidc.signIn', { providerName: 'IDONE' }) }}
      </button>
    </div>
  </main>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { buildOAuthLoginStartURL } from '@/api/auth'
import { useAppStore } from '@/stores'
import { resolveAffiliateReferralCode, storeOAuthAffiliateCode } from '@/utils/oauthAffiliate'

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()
const isLoading = ref(false)
const errorKey = ref('')

onMounted(() => {
  // An error returned to this entry point requires an explicit retry.
  if (route.query.error || route.query.error_description || route.query.error_message) {
    errorKey.value = 'auth.oidc.startFailed'
    return
  }
  void startLogin()
})

function safeRedirect(): string {
  const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : ''
  const hasUnsafeCharacters = Array.from(redirect).some(char => char === '\\' || char.charCodeAt(0) <= 32)
  return redirect.startsWith('/') && !redirect.startsWith('//') && !hasUnsafeCharacters
    ? redirect
    : '/dashboard'
}

async function startLogin(force = false): Promise<void> {
  if (isLoading.value) return
  isLoading.value = true
  errorKey.value = ''

  try {
    const settings = await appStore.fetchPublicSettings(force)
    if (!settings) {
      errorKey.value = 'auth.oidc.settingsFailed'
      return
    }
    if (!settings.oidc_oauth_enabled) {
      errorKey.value = 'auth.oidc.unavailable'
      return
    }

    storeOAuthAffiliateCode(resolveAffiliateReferralCode(undefined, route.query.aff, route.query.aff_code))
    // Replace the entry page so Back does not immediately start authentication again.
    window.location.replace(buildOAuthLoginStartURL({
      provider: 'oidc',
      params: { redirect: safeRedirect() }
    }))
  } catch {
    errorKey.value = 'auth.oidc.startFailed'
  } finally {
    isLoading.value = false
  }
}
</script>
