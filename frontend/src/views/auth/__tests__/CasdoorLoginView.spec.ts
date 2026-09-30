import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import CasdoorLoginView from '../CasdoorLoginView.vue'
import { loadOAuthAffiliateCode } from '@/utils/oauthAffiliate'

const { fetchSettings, replace, query } = vi.hoisted(() => ({
  fetchSettings: vi.fn(), replace: vi.fn(), query: {} as Record<string, string>
}))
vi.mock('vue-router', () => ({ useRoute: () => ({ query }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ fetchPublicSettings: fetchSettings }) }))
vi.mock('@/api/auth', () => ({
  buildOAuthLoginStartURL: ({ params }: { params: Record<string, string> }) =>
    `/api/v1/auth/oauth/oidc/start?${new URLSearchParams(params)}`
}))

beforeEach(() => {
  vi.clearAllMocks()
  Object.keys(query).forEach(key => delete query[key])
  fetchSettings.mockResolvedValue({ oidc_oauth_enabled: true })
  vi.stubGlobal('location', { replace })
  sessionStorage.clear()
  localStorage.clear()
})
afterEach(() => vi.unstubAllGlobals())

describe('IDONE login entry', () => {
  it('waits for settings then redirects, preserving the destination and referral', async () => {
    let resolve!: (value: unknown) => void
    fetchSettings.mockReturnValue(new Promise(r => { resolve = r }))
    query.redirect = '/keys?tab=active'
    query.aff = 'referral123'
    const wrapper = mount(CasdoorLoginView)
    expect(replace).not.toHaveBeenCalled()
    expect(wrapper.find('[role="status"]').exists()).toBe(true)
    resolve({ oidc_oauth_enabled: true })
    await flushPromises()
    expect(replace).toHaveBeenCalledTimes(1)
    expect(replace).toHaveBeenCalledWith('/api/v1/auth/oauth/oidc/start?redirect=%2Fkeys%3Ftab%3Dactive')
    expect(loadOAuthAffiliateCode()).toBe('referral123')
    wrapper.unmount()
  })
  it.each(['https://example.com', '//example.com', '/\\example.com', '/\n/example.com', ''])(
    'rejects unsafe or empty redirect %j', async redirect => {
      query.redirect = redirect
      const wrapper = mount(CasdoorLoginView)
      await flushPromises()
      expect(replace).toHaveBeenCalledWith('/api/v1/auth/oauth/oidc/start?redirect=%2Fdashboard')
      wrapper.unmount()
    }
  )
  it.each([null, { oidc_oauth_enabled: false }])('allows retry when settings are unavailable: %j', async settings => {
    fetchSettings.mockResolvedValueOnce(settings)
    const wrapper = mount(CasdoorLoginView)
    await flushPromises()
    expect(replace).not.toHaveBeenCalled()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(fetchSettings).toHaveBeenLastCalledWith(true)
    expect(replace).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
  it('handles rejected settings requests', async () => {
    fetchSettings.mockRejectedValueOnce(new Error('offline'))
    const wrapper = mount(CasdoorLoginView)
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('auth.oidc.startFailed')
    expect(replace).not.toHaveBeenCalled()
    wrapper.unmount()
  })
  it('requires explicit retry when returning with an authentication error', async () => {
    query.error = 'access_denied'
    const wrapper = mount(CasdoorLoginView)
    await flushPromises()
    expect(fetchSettings).not.toHaveBeenCalled()
    expect(replace).not.toHaveBeenCalled()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(replace).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
})
