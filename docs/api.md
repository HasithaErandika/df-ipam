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

Standard CRUD (`GET` list, `POST`, `GET /:id`, `PATCH /:id`, `DELETE /:id`) applies to: `/business-units`, `/sites`, `/network-zones`, `/vdoms`, `/vlans`, `/network-blocks`, `/devices`, `/interfaces`, `/business-services`, `/applications`, `/teams`, `/reserved-ranges`, `/dhcp-scopes`, `/overlap-exceptions`. They share one handler, `modules/catalog`. `/organisations` and `/routing-domains` have list and create.

| Area | Endpoints |
| --- | --- |
| Auth | `GET /auth/me`, `GET /auth/me/profile` (sign-in itself happens in Supabase) |
| Users and roles | `/users`, `/users/invite`, `/users/:id/roles`, `/roles`, `GET /people` (email and name only, for owner pickers) |
| Subnets | CRUD on `/subnets`, plus `POST /subnets/:id/archive`, `POST /subnets/bulk` (all or nothing, `dryRun`), `POST /subnets/bulk-archive`, `POST /subnets/check-overlap`, `POST /subnets/calculate`, `GET /subnets/:id/utilisation`, `GET /subnets/:id/free-ranges`, `GET /subnets/:id/addresses?offset=&limit=` |
| Tools | `POST /tools/split`, `POST /tools/summarise`, `GET /tools/locate?address=`, `GET /network-blocks/:id/free-space` |
| IP addresses | `GET /ip-addresses`, `POST /ip-addresses`, `GET /ip-addresses/:id`, `PATCH /ip-addresses/:id`, `GET /ip-addresses/:id/history`, `GET /ip-addresses/:id/evidence`, `GET /ip-addresses/:id/device` (inventory and read-only sources only) |
| DNS and DHCP (read-only inventory) | `GET /dns-records`, `GET /dhcp-scopes`, `GET /dhcp-scopes/:id` |
| Imports enrichment | `GET /imports/enrichment-queue`, `PATCH /imports/records/:id` |
| Requests | CRUD on `/requests` (edit and delete drafts only, by the requester), plus `POST /requests/:id/submit`, `POST /requests/:id/cancel`. Without a wider permission, people see only their own |
| Approvals | `GET /approvals` (steps the caller may decide now), `POST /approvals/:id/approve`, `POST /approvals/:id/reject`; `GET`, `POST`, `PUT /:id`, `DELETE /:id` on `/approval-workflows` |
| Allocations | `GET /allocations`, `GET /allocations/:id`, `POST /allocations` and `POST /allocations/:id/release` (fulfil an approved request; repeats return the same result) |
| Reservations | `GET /reservations`, `GET /reservations/:id`, `POST /reservations` (fulfil an approved request), `POST /reservations/:id/extend`, `POST /reservations/:id/release`. History is kept, so there is no delete |
| Discovery | `/discovery/policies`, `/discovery/schedules`, `/discovery/exclusions`, `/discovery/jobs`, `POST /discovery/jobs/:id/{pause,resume,cancel}`, `POST /discovery/emergency-stop` |
| Agents (admin) | `/agents`, `POST /agents/:id/{revoke,rotate}` |
| Agents (agent credential) | `POST /agent-api/register`, `POST /agent-api/heartbeat`, `GET /agent-api/jobs`, `POST /agent-api/jobs/:id/results`. A job's `maxPps` is what is left of the ceiling, not the policy's figure. The reply to results holds `instruction` (`continue`, `pause` or `stop`) and `skipTargets` |
| Reconciliation | `GET /reconciliation/results`, `GET /reconciliation/summary`, `GET /reconciliation/last-run`, `POST /reconciliation/run`, `POST /reconciliation/results/:id/review`, `/reconciliation/rules` |
| Imports | `POST /imports`, `GET /imports/:id/preview`, `POST /imports/:id/{approve,reverse}`, `GET /imports` |
| Notifications | `GET /notifications`, `GET /notifications/unread-count`, `POST /notifications/:id/read`, `POST /notifications/read-all`, `GET`/`PUT /notifications/preferences` (own), `GET /notifications/rules`, `PUT /notifications/rules/:eventType` |
| Audit | `GET /audit-logs`, `GET /audit-logs/filters` |
| Dashboard and search | `GET /dashboard/summary`, `GET /dashboard/trends`, `GET /search?q=`, `GET`/`POST /saved-filters`, `DELETE /saved-filters/:id` (own) |

## Key error codes

`VALIDATION_FAILED`, `UNAUTHENTICATED`, `REQUEST_NOT_APPROVED`, `SEPARATION_OF_DUTIES`, `NOT_ELIGIBLE_APPROVER`, `APPROVAL_NOT_CURRENT`, `RELEASE_ALREADY_REQUESTED`, `RESERVATION_NOT_ACTIVE`, `FORBIDDEN`, `NOT_FOUND`, `STALE_VERSION`, `SUBNET_OVERLAP`, `ADDRESS_NOT_IN_SUBNET`, `ADDRESS_UNAVAILABLE`, `SUBNET_EXHAUSTED`, `SELF_APPROVAL_DENIED`, `TARGET_NOT_ALLOWED`, `DISCOVERY_STOPPED`, `RATE_LIMITED`, `INTERNAL_ERROR`.

## Phases 5 to 7

| Area | Routes | Permission |
| --- | --- | --- |
| Integrations | `/integrations/connections`, `/{id}/test`, `/{id}/sync`, `/{id}/syncs`, `/integrations/syncs/{id}/errors`, `/integrations/kinds` | `integrations:manage`; reading also `audit:view` |
| Rule settings | `/reconciliation/rules`, `/reconciliation/rules/{classification}` | `inventory:view`; `discovery:manage` to change |
| Change requests | `/reconciliation/results/{id}/change-requests`, `/change-requests`, `/{id}/approve`, `/{id}/reject` | `inventory:edit` to propose, `requests:approve` to decide |
| Reports | `/reports`, `/reports/types`, `/reports/summary`, `/reports/{id}/download` | `reports:export` or `reports:export_scoped`. Audit reports need `audit:view` |
| Dashboard | `/dashboard/trends`, `/dashboard/distribution` | `inventory:view` |
| Roles | `/admin/permission-matrix`, `/roles`, `/roles/{id}`, `/roles/{id}/permissions` | `users:manage` |
| System | `/system/health`, `/system/retention`, `/system/retention/{class}` | `system:manage`; reading also `audit:view` |

`/users`, `/integrations/connections` and `/reconciliation/results` take `q` to search. `/audit-logs` takes `actor`, `action`, `objectType`, `result`, `from` and `to`, and `/audit-logs/filters` lists the actions and object types that exist.

New error codes: `ENCRYPTION_NOT_CONFIGURED`, `CONNECTOR_NOT_AVAILABLE`, `CREDENTIALS_MISSING`, `CONNECTION_HAS_HISTORY`, `SYNC_ACTIVE`, `NAME_TAKEN`, `FINDING_NOT_CHANGEABLE`, `CHANGE_ALREADY_OPEN`, `CHANGE_ALREADY_DECIDED`, `REPORT_LIMIT`, `REPORT_NOT_READY`, `ROLE_PROTECTED`, `LAST_ADMINISTRATOR`, `ROLE_IN_USE`, `ROLE_IS_SYSTEM`.
