# kena

Gap-fill trainer for Modern Greek grammar. A sentence has blanks (`___`), you
type the missing forms, the server checks them strictly (accents count).

kena never generates exercises. An agent does: you show Claude or ChatGPT a photo
of a textbook exercise, it writes new tasks on the same grammar point and uploads
them through kena's MCP server. The MCP server also hands the agent an authoring
guide (`get_authoring_guide`) so the tasks are unambiguous and level-appropriate.

- Web: `/` lists sets, `/s/<slug>` practises one. Cards mode (one task per
  screen, auto-advance) or List mode. Filters: all, unsolved, mistakes. No
  login; progress is tied to an anonymous device cookie.
- MCP: `/mcp` (Streamable HTTP). Personal bearer tokens or OAuth (for
  claude.ai and ChatGPT connectors). You can only edit your own sets; admins can
  edit everything and manage users and tokens.

## Run locally

Needs Go and a Postgres.

```
export KENA_DATABASE_URL="postgres://localhost/kena"
export KENA_PUBLIC_URL="http://127.0.0.1:8080"
go run ./cmd/kena token create --user anton --admin   # prints a token once
go run ./cmd/kena serve
```

| env                 | default          |                                                        |
| ------------------- | ---------------- | ------------------------------------------------------ |
| `KENA_DATABASE_URL` | required         | pgx connection string                                  |
| `KENA_PUBLIC_URL`   | required (serve) | external base URL; OAuth metadata and set links use it |
| `KENA_LISTEN`       | `127.0.0.1:8080` |                                                        |

Migrations run on `serve`, `migrate` and `token create`.

## NixOS

```nix
# flake inputs
kena.url = "github:pltanton/kena";

# configuration
imports = [inputs.kena.nixosModules.default];
services.kena = {
  enable = true;
  publicUrl = "https://kena.example.org";
};
services.caddy.virtualHosts."kena.example.org".extraConfig = ''
  reverse_proxy 127.0.0.1:8080
'';
```

The module creates a local Postgres database and a `kena` system user. Tokens:

```
sudo kena-admin token create --user anton --admin
```

## Connecting agents

Any token can be issued later from an admin agent with `admin_issue_token`.

- **Claude Code:**
  `claude mcp add --transport http kena https://kena.example.org/mcp --header "Authorization: Bearer kena_…"`
- **Hermes / other agents:** Streamable HTTP endpoint `https://kena.example.org/mcp`,
  header `Authorization: Bearer kena_…`.
- **claude.ai:** Settings → Connectors → Add custom connector → URL
  `https://kena.example.org/mcp`. On the kena page that opens, paste your token.
- **ChatGPT:** Settings → Apps → developer mode → create app with the same URL,
  authentication OAuth. Paste your token on the kena page.

Example prompt with a photo attached:

> This is exercise 3 from unit 5 of my B1 book. Make 100 more tasks on the
> same grammar point and upload them to kena.

The agent reads the guide, writes new sentences in batches, validates them and
returns the set URL.

## Development

See `AGENTS.md`. Design: `docs/superpowers/specs/2026-10-16-kena-design.md`.
