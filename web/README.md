# DF-IPAM Web

The React frontend: TypeScript, Vite, Tailwind CSS, React Router and TanStack Query. Run commands from the repository root with `make` (see `make help`), or use `pnpm` in this folder.

## Setup

Copy `.env.example` to `.env` and fill in the Supabase URL and anon key. `VITE_API_URL` points at the API, which must list this app's origin in `CORS_ALLOWED_ORIGINS`.

## Layout

```text
web/src/
├── lib/
│   ├── auth.ts       # the only file that imports supabase-js
│   └── api.ts        # API client
├── components/       # shared UI
├── layouts/          # app shell
└── pages/            # one folder or file per page
```

## What is built

Sign-in, Dashboard, Network Management, Subnet Management, User Management and Audit Logs work against the API. The other pages state their roadmap phase and show no sample data. Navigation lives in `src/lib/nav.ts`.

## Rules

- Keep Supabase inside `src/lib/auth.ts`, so moving to company auth stays a one-file change.
- Show allocation state and observation state as separate badges.
- Address counts arrive as strings. Don't convert them to numbers, because IPv6 counts exceed safe integers.
- Never render a full address range. Use server-side pagination and range loading.

See [docs/frontend.md](../docs/frontend.md) for pages and design rules.
