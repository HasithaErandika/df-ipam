# API

REST under `/api/v1`. The contract is `api/openapi.yaml`; a test fails if a route is missing from it.

## Conventions

| Topic | Rule |
| --- | --- |
| Auth | The session cookie on every `/api/v1` route, and `X-Requested-With: DF-IPAM` on every change. `/auth/*` sign-in routes, `/health` and `/ready` are open |
| Permissions | Every route is registered with its permissions through the route guard. A test fails on an unprotected route |
| Lists | `?page=1&pageSize=25`, at most 200. Ordering always ends in a unique column, so pages never overlap |
| Sorting and filters | Named query parameters, allowlisted per endpoint |
| Responses | DTOs only, never database rows, never secrets |
| Correlation | `X-Correlation-ID` is accepted or generated, and returned on every response |

**Success**

```json
{ "data": { } }
{ "data": [ ], "meta": { "page": 1, "pageSize": 25, "total": 240, "totalPages": 10, "hasNext": true, "hasPrev": false } }
```

**Error**, with a stable code:

```json
{ "error": { "code": "VALIDATION_FAILED", "message": "…", "details": [{ "field": "ip", "issue": "…" }], "correlationId": "…" } }
```

Unknown errors are logged with the correlation ID and answered as `INTERNAL_ERROR`, with no detail.

| Status | Use |
| --- | --- |
| 400 | Malformed request |
| 401 / 403 | Not signed in / not permitted |
| 404 | Not found (a malformed ID is also not found) |
| 409 | Conflict, such as an IP already registered or a scan already running |
| 422 | Validation failed, with `details` per field |
| 429 | Rate limited, or a manual scan within a minute of the last |
| 503 | The scanner isn't running in this process |

## Limits

| Limit | Value |
| --- | --- |
| Request body | 1 MB; imports 5 MB and 5,000 rows |
| Imports | `?dryRun=true` does the whole import in a transaction that is rolled back, and returns what would be added and skipped |
| Per client address | 50 requests a second, bursts of 100 |
| Per signed-in person | 30 requests a second, bursts of 60 |
| Requests running at once | 200 (`API_MAX_IN_FLIGHT`); more are refused, not queued |

Nginx in the `web` container adds its own per-address limits.

## Endpoint groups

| Group | Paths |
| --- | --- |
| Account | `/auth/me`, `/auth/me/profile` |
| Segments | `/segments`, `/segments/{id}`, `/segments/import` (text), `/segments/{id}/addresses`, `/segments/{id}/map` |
| Devices | `/devices`, `/devices/{id}`, `/devices/{id}/decision`, `/devices/import`, `/devices/export` |
| IP register | `/ip-addresses`, `/ip-addresses/{id}`, `/ip-addresses/available-candidate`, `/ip-addresses/check`, `/ip-addresses/import` (`?as=server`), `/ip-addresses/export` |
| Discovery | `/discovery/status`, `/discovery/scan`, `/discovery/findings`, `/discovery/summary` |
| Requests and reservations | `/ip-requests…`, `/reservations…` |
| Alerts and notifications | `/alerts…`, `/notifications…`, `/notification-preferences` |
| Reports and audit | `/reports…`, `/audit-logs`, `/audit-logs/export` |
| Administration | `/users…`, `/roles`, `/roles/{key}`, `/environments…`, `/departments…`, `/settings` |
| FortiGate | `/integrations/fortigate`, `…/test`, `…/sync` |

Every change to devices, the register or segments re-checks the alert rules before replying, so the reply already reflects closed alerts.
