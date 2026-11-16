# DF-IPAM Web

The React frontend: TypeScript, Vite, Tailwind CSS, React Router and TanStack Query. Run commands from the repository root with `make` (see `make help`), or `pnpm` in this folder.

## Setup

Copy `.env.example` to `.env`. Leave `VITE_API_URL` empty, so the app and the API share one address and the session cookie is sent.

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

Every screen in [docs/product/screens.md](../docs/product/screens.md#screens) is built and works against the API. Navigation and who sees what are in `src/app/nav.ts`.

## Rules

- Keep sign-in inside `src/lib/auth.ts`. The browser never holds a token: the API's cookie signs every request.
- Show register state and evidence as separate badges. Say "Available candidate", never "Free".
- Light theme only; keep the colour tokens in `index.css`.
- No fake data: screens show real records or say what is missing.

See [docs/frontend.md](../docs/engineering/frontend.md) for design and UI rules.

## Container

`web/Dockerfile` builds the app and serves it from Nginx, with the settings in `web/nginx`. `make web` runs Vite for development.
