import { type ReactNode } from 'react'
import {   type Issue } from '../../lib/validation'

/* eslint-disable @typescript-eslint/no-explicit-any */
export type Row = Record<string, any> & { id: string }

export interface FieldDef {
  key: string
  label: string
  type: 'text' | 'number' | 'bool' | 'cidr' | 'ip' | 'ref' | 'select' | 'date' | 'datetime' | 'timezone'
  required?: boolean
  /** For ref fields: the list endpoint, and how to name an option. */
  ref?: string
  refLabel?: (o: Row) => string
  options?: Record<string, string>
  hint?: string
  /** Fixed once created, because other records depend on it. */
  createOnly?: boolean
  min?: number
  max?: number
  /** An extra check on a filled-in value. */
  check?: (value: string) => Issue
}

interface ColumnDef { label: string; render: (r: Row) => ReactNode; className?: string }

export interface EntityDef {
  /** The field naming who owns a record. Its owner may edit it with inventory:edit_assigned, apart from ownerLocked. */
  ownerKey?: string
  ownerLocked?: string[]
  key: string
  label: string
  singular: string
  path: string
  description: string
  fields: FieldDef[]
  columns: ColumnDef[]
  /** Permission to write, when it isn't the inventory permissions. */
  writePermission?: string
  readOnly?: boolean
  searchLabel?: string
  /** Checks across fields, such as an end after a start. */
  validate?: (form: Record<string, string | boolean>) => Record<string, Issue>
  rowAction?: (r: Row) => ReactNode
}
