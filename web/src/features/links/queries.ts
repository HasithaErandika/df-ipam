import { useQuery } from '@tanstack/react-query'
import { get } from '../../lib/api'
import type { LinkOptions, LinkProvider, LinkSettings } from '../../lib/types'

/** Every query the links screens refresh after a change. */
export const linkKeys = ['/network-links', '/link-providers', '/link-settings']

export const useLinkSettings = () => useQuery({ queryKey: ['/link-settings'], queryFn: () => get<LinkSettings>('/link-settings'), staleTime: 60_000 })

export const useProviders = () => useQuery({ queryKey: ['/link-providers'], queryFn: () => get<LinkProvider[]>('/link-providers'), staleTime: 30_000 })

export const useLinkOptions = (enabled = true) => useQuery({
  queryKey: ['/network-links', 'options'], queryFn: () => get<LinkOptions>('/network-links/options'), staleTime: 60_000, enabled,
})
