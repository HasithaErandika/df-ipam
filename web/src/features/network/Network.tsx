import { useRouteTab } from '../../lib/tabs'
import { useState } from 'react'
import { CatalogManager } from '../../components/catalog/CatalogManager'
import { entities } from '../../components/catalog/entities'
import { type Row } from '../../components/catalog/types'
import { Button, PageHeader } from '../../components/ui'
import { RouteTabs } from '../../components/workflow'
import { Hierarchy } from './Hierarchy'
import { BlockSpaceModal } from './modals'

export function Network() {
  const [space, setSpace] = useState<Row | null>(null)
  const defs = entities.map((e) => e.key === 'blocks'
    ? { ...e, rowAction: (r: Row) => <Button variant="ghost" className="h-7" onClick={() => setSpace(r)}>Free space</Button> }
    : e)
  const tabs = [{ key: 'hierarchy', label: 'Organisations and routing' }, ...defs.map((e) => ({ key: e.key, label: e.label }))]
  const tab = useRouteTab(tabs.map((t) => t.key))
  const def = defs.find((e) => e.key === tab)
  return (
    <>
      <PageHeader title="Network Management" description="The hierarchy that gives each address its context: organisations, routing domains, sites, zones, VDOMs, VLANs, blocks and owners." />
      <RouteTabs tabs={tabs} active={tab} base="/network" />
      {tab === 'hierarchy' ? <Hierarchy /> : def && <CatalogManager key={def.key} def={def} />}
      {space && <BlockSpaceModal block={space} onClose={() => setSpace(null)} />}
    </>
  )
}
