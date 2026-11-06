import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { list } from '../../lib/api'
import { type Paging } from '../../lib/paging'
import type { Organisation, RoutingDomain } from '../../lib/types'

export const useOrganisations = (p: Paging) => useQuery({
  queryKey: ['organisations', p.page, p.pageSize], placeholderData: keepPreviousData,
  queryFn: () => list<Organisation>(`/organisations?page=${p.page}&pageSize=${p.pageSize}`),
})

export const useDomains = (p: Paging) => useQuery({
  queryKey: ['routing-domains', p.page, p.pageSize], placeholderData: keepPreviousData,
  queryFn: () => list<RoutingDomain>(`/routing-domains?page=${p.page}&pageSize=${p.pageSize}`),
})
