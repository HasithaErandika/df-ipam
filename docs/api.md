# API Design

Versioned REST under `/api/v1`. The contract lives in `api/openapi.yaml`, and Swagger UI is served outside production.

## Conventions

**List response**

```json
{
  "data": [],
  "meta": { "page": 1, "pageSize": 50, "total": 1240, "totalPages": 25, "hasNext": true, "hasPrev": false }
}
```

**Error response**

```json
{
  "error": {
    "code": "SUBNET_OVERLAP",
    "message": "This subnet overlaps 10.0.0.0/24 in the same routing domain.",
    "details": [{ "field": "cidr", "issue": "overlap" }],
    "correlationId": "01J..."
  }
}
```

| Topic | Rule |
| --- | --- |
| Auth | `Authorization: Bearer <token>` on every route except `/health` |
| Pagination | `?page=1&pageSize=50`, maximum 200, on every list. Ordering always ends in a unique column, so pages never overlap. A page past the end returns no rows, with `totalPages` showing where to go. Audit logs use the same scheme for now; move them to cursors if volume requires it |
| Sorting | `?sort=-updatedAt,name`, allowlisted per endpoint |
| Filtering | Named query parameters, allowlisted per endpoint |
| Correlation | `X-Correlation-ID` accepted or generated, and returned on every response |
| Concurrency | Updates send `If-Match` with the record version, otherwise `412` |
| Idempotency | `Idempotency-Key` supported on allocation and job-start endpoints |
| Large numbers | Address counts are JSON **strings**, because IPv6 values exceed safe integers |
| DTOs | Database rows are never returned directly. Secrets are never returned |

## Status codes

| Code | Use |
| --- | --- |
| 400 | Malformed request |
| 401 / 403 | Not authenticated / not permitted |
| 404 | Not found, or outside the user's scope |
| 409 | Business conflict: overlap, address already allocated |
| 412 | Stale version |
| 422 | Validation failed |
| 429 | Rate limited |

## Endpoint map

Standard CRUD (`GET` list, `POST`, `GET /:id`, `PATCH /:id`, `DELETE /:id`) applies to: `/business-units`, `/sites`, `/network-zones`, `/vdoms`, `/vlans`, `/network-blocks`, `/devices`, `/interfaces`, `/business-services`, `/applications`, `/teams`, `/reserved-ranges`, `/dhcp-scopes`, `/overlap-exceptions`. They share one handler, `modules/catalog`. `/organisations` and `/routing-domains` have list, create, get, edit and delete.

| Area | Endpoints |
| --- | --- |
| Auth | `GET /auth/me` (sign-in itself happens in Supabase) |
| Users and roles | `/users`, `/users/:id/roles`, `/roles`, `/permissions`, `GET /users/lookup` (owner picker, needs only `inventory:view`) |
| Subnets | CRUD on `/subnets`, plus `POST /subnets/:id/archive`, `POST /subnets/bulk`, `POST /subnets/check-overlap`, `POST /subnets/calculate`, `GET /subnets/:id/utilisation`, `GET /subnets/:id/free-ranges`, `GET /subnets/:id/addresses?offset=&limit=` |
| Tools | `POST /tools/split`, `POST /tools/summarise`, `GET /tools/locate?address=`, `GET /network-blocks/:id/free-space` |
| IP addresses | `GET /ip-addresses`, `POST /ip-addresses`, `GET /ip-addresses/:id`, `PATCH /ip-addresses/:id`, `GET /ip-addresses/:id/history`, `GET /ip-addresses/:id/evidence`, `POST /ip-addresses/bulk` |
| DNS and DHCP (read-only inventory) | `GET /dns-records`, `GET /dhcp-reservations`, `GET /dhcp-scopes/:id/reservations` |
| Imports enrichment | `GET /imports/enrichment-queue`, `PATCH /imports/records/:id` |
| Requests | CRUD on `/requests`, plus `POST /requests/:id/submit`, `POST /requests/:id/cancel` |
| Approvals | `GET /approvals`, `POST /approvals/:id/approve`, `POST /approvals/:id/reject` |
| Allocations | `GET /allocations`, `POST /allocations`, `POST /allocations/:id/release` |
| Reservations | CRUD on `/reservations`, plus `POST /reservations/:id/extend` |
| Discovery | `/discovery/policies`, `/discovery/schedules`, `/discovery/exclusions`, `/discovery/jobs`, `POST /discovery/jobs/:id/{pause,resume,cancel}`, `POST /discovery/emergency-stop` |
| Agents (admin) | `/agents`, `POST /agents/:id/{revoke,rotate}` |
| Agents (agent credential) | `POST /agent-api/register`, `POST /agent-api/heartbeat`, `GET /agent-api/jobs`, `POST /agent-api/jobs/:id/results` |
| Reconciliation | `GET /reconciliation/results`, `GET /reconciliation/summary`, `POST /reconciliation/run`, `POST /reconciliation/results/:id/review`, `/reconciliation/rules` |
| Integrations | CRUD on `/integrations`, plus `POST /integrations/:id/{test,sync}`, `GET /integrations/:id/sync-jobs` |
| Imports | `POST /imports`, `GET /imports/:id/preview`, `POST /imports/:id/{approve,reverse}`, `GET /imports` |
| Notifications | `GET /notifications`, `POST /notifications/:id/read`, `/notifications/rules`, `/notifications/preferences` |
| Reports | `GET /reports/types`, `POST /reports`, `GET /reports/:id`, `GET /reports/:id/download` |
| Audit | `GET /audit-logs`, `GET /audit-logs/:id` |
| Dashboard and search | `GET /dashboard/summary`, `GET /dashboard/trends`, `GET /search?q=` |
| Saved filters | `GET /saved-filters?page=`, `POST /saved-filters`, `DELETE /saved-filters/:id`. Each user sees only their own |
| Administration | `/administration/settings`, `GET /administration/health` |

## Bulk changes

`POST /subnets/bulk` takes `ids` (up to 500), an `action` (`update`, `archive` or `delete`) and, for updates, the same `changes` as a subnet edit except name and gateway. `POST /ip-addresses/bulk` takes `ids` and `changes`; hostname, FQDN and MAC are per record. Both run in one transaction: if any record fails, nothing changes and the error names that record. Each record still gets its own audit row.

## Key error codes

`VALIDATION_FAILED`, `ALREADY_EXISTS`, `RELATED_RECORD`, `SUBNET_IN_USE`, `SUBNET_ARCHIVED`, `UNAUTHENTICATED`, `FORBIDDEN`, `NOT_FOUND`, `STALE_VERSION`, `SUBNET_OVERLAP`, `ADDRESS_NOT_IN_SUBNET`, `ADDRESS_UNAVAILABLE`, `SUBNET_EXHAUSTED`, `SELF_APPROVAL_DENIED`, `TARGET_NOT_ALLOWED`, `DISCOVERY_STOPPED`, `RATE_LIMITED`, `INTERNAL_ERROR`.
