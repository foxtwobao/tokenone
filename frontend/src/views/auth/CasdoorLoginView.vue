<template>
  <AuthLayout>
    <div class="space-y-6">
      <div class="text-center">
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('auth.welcomeBack') }}
        </h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
          {{ t('auth.signInToAccount') }}
        </p>
      </div>

      <div class="space-y-3">
        <OidcOAuthSection
          :disabled="!settingsReady || !oidcEnabled || isLoading"
          provider-name="IDONE"
          :show-divider="false"
          @start="handleOAuthStart"
        />
        <p v-if="!settingsReady" class="text-center text-sm text-gray-500 dark:text-dark-400">
          {{ t('common.loading') }}
        </p>
      </div>
    </div>
  </AuthLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { AuthLayout } from '@/components/layout'
import OidcOAuthSection from '@/components/auth/OidcOAuthSection.vue'
import { buildOAuthLoginStartURL, type OAuthLoginStart } from '@/api/auth'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()

const settingsReady = ref(false)
const isLoading = ref(false)

const oidcEnabled = computed(() => appStore.cachedPublicSettings?.oidc_oauth_enabled === true)

onMounted(async () => {
  await appStore.fetchPublicSettings()
  settingsReady.value = true
})

function safeRedirect(): string {
  const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : ''
  return redirect.startsWith('/') && !redirect.startsWith('//') ? redirect : '/dashboard'
}

function handleOAuthStart(request: OAuthLoginStart): void {
  if (!oidcEnabled.value || isLoading.value) return

  isLoading.value = true
  window.location.assign(
    buildOAuthLoginStartURL({
      ...request,
      provider: 'oidc',
      params: { ...request.params, redirect: safeRedirect() }
    })
  )
}
</script>
