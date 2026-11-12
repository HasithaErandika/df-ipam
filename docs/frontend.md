# Frontend

React, TypeScript, Vite, Tailwind CSS, Radix primitives, React Router and TanStack Query. Forms use React Hook Form with Zod, or the shared checks in `lib/validation.ts`. Charts are hand-built SVG. `supabase-js` is used only in `web/src/lib/auth.ts`.

## Design

- **Light theme only,** blue, with colour tokens in `web/src/index.css`. No dark theme or theme toggle.
- Compact tables, monospaced IPs, MACs and CIDRs, restrained colour. Status always has text, not colour alone.
- Platform marks from Simple Icons (CC0); Windows is drawn as a plain four-pane window, since Simple Icons has no Microsoft marks. A guessed platform shows faded and labelled "likely".

### Status language

The register and the evidence are always **separate badges**.

| Group | Values |
| --- | --- |
| Register | Assigned, Reserved, Blocked, Not registered, Registered elsewhere |
| Evidence | Online, Unavailable since …, New |
| Device decision | Pending, Approved, Rejected |

Say **Available candidate**, never "Free".

## Screens

One folder per screen in `web/src/features/`: `dashboard`, `discovery`, `servers`, `devices`, `alerts`, `requests`, `reservations`, `reports`, `audit`, `network` (Network setup), `access` (Users and roles), `system`, plus `auth` and `shared`. What each screen holds is in [new.md](new.md#screens). Screens not built say so, and no screen shows sample data.

## Patterns

- **Dialogs** are centred over a blurred background (`Modal`, `ConfirmDialog`). No side drawers.
- **Tabs are paths** (`/discovery/segments`, `/network-setup/departments`) through `PathTabs` and `useRouteTab`. Old `/settings/…` links redirect to the page that now holds them.
- **The sidebar** groups pages, with the collapse button at the top, counts beside Devices (pending), Alerts (open) and IP Requests (waiting, for approvers), and the version and build at the bottom (`__APP_VERSION__` from `package.json`, `__APP_COMMIT__` from git, passed in by `make` for Docker builds).
- **The header** shows the notification bell, who is signed in with their role (click for details), and Sign out.
- **Drill-down pages** (a segment) link back to the menu page they were opened from (`lib/backTo.ts`), with its filters.
- **Imports** are reviewed before they run: a dry run shows every row to be added and every skipped row with its reason (`ImportPreview`, `SegmentImport`).
- **The query string** holds filters, sorting, paging and open dialogs (`?finding=<id>`).
- **Every data view** has loading, empty and error states (`DataTable` does this).
- **Tables** page on the server: 25 rows by default, up to 200. Each row shows its one main action as a button, and the rest in a menu.
- **Long forms** keep the essentials visible and the rest under `MoreOptions`, which opens by itself if a hidden field has an error.

## Feedback

- **Success:** every mutation has `meta.success`; `app/queryClient.ts` shows it as a message. `meta.inlineError` keeps the error in the form instead of a message.
- **Confirming:** ask before every delete, destructive action and edit save, with `useConfirm` or `useConfirmSave`. Pass the list of changes (`lib/changes.ts`) so the dialog shows exactly what will change. Keep Save disabled until something has changed. Creating a record doesn't ask.
- **Errors:** show `errorMessage(error)`, never a raw exception. A 401 signs the person out; 4xx requests aren't retried.

## Code

- `app/` router (each screen loaded on first use), shell, navigation (`nav.ts` holds who sees what), query client.
- `components/ui/` the design system. `lib/` non-visual code: API client, auth, formats, labels, platforms, types.
- Keep files under 200 lines.

## Accessibility

Target WCAG 2.2 AA: keyboard use, visible focus, labelled fields with errors linked to them (`Field`), and enough contrast.
