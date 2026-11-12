# DF-IPAM Web

The React frontend: TypeScript, Vite, Tailwind CSS, React Router and TanStack Query. Run commands from the repository root with `make` (see `make help`), or `pnpm` in this folder.

## Setup

Copy `.env.example` to `.env` and fill in the Supabase URL and anon key. `VITE_API_URL` points at the API, which must list this app's origin in `CORS_ALLOWED_ORIGINS`.

## Layout

```text
web/src/
├── app/           # router (lazy screens), navigation, shell, query client
├── components/    # ui/ design system, platform icons, subnet map, charts
├── features/      # one folder per screen
├── lib/           # API client, auth, formats, labels, platforms, types
├── index.css      # colour tokens
└── main.tsx
```

Every screen in [docs/new.md](../docs/new.md#screens) is built and works against the API. Navigation and who sees what are in `src/app/nav.ts`.

## Rules

- Keep Supabase inside `src/lib/auth.ts`, so moving to company sign-in stays a small change.
- Show register state and evidence as separate badges. Say "Available candidate", never "Free".
- Light theme only; keep the colour tokens in `index.css`.
- No fake data: screens show real records or say what is missing.

See [docs/frontend.md](../docs/frontend.md) for design and UI rules.

## Container

`web/Dockerfile` builds the app and serves it from Nginx, with the settings in `web/nginx`. `make web` runs Vite for development.
