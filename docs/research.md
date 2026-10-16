# Research notes (2026-10-16)

Collected by research agents from search results; URLs are pointers, not all read in full.

## MCP clients

- Skills over MCP: SEP-2640 `io.modelcontextprotocol/skills` is Final (2026-09-13),
  `skills/list`/`skills/get`, files as `skill://` resources. No Claude product
  consumes it at runtime; ChatGPT imports a static snapshot at plugin submission
  only. Go SDK support is open (PR #1238/#1240).
  https://modelcontextprotocol.io/seps/2640-skills-extension
- `instructions` from initialize: used by ChatGPT/Codex (first 512 chars matter)
  and Claude Code; dropped by claude.ai (anthropics/claude-ai-mcp#93).
- Prompts/resources in claude.ai are user-selected only (claude-ai-mcp#23).
- Conclusion: a `get_authoring_guide` tool is the only channel every client's model sees.

## OAuth for connectors

claude.ai / Claude Code (https://claude.com/docs/connectors/building/authentication.md):

- 401 + `WWW-Authenticate: Bearer resource_metadata="…"`; header ignored on 200.
- RFC 9728 `resource` must equal the MCP URL exactly, path included. Only the
  first `authorization_servers` entry is used.
- RFC 8414 or OIDC metadata required.
- DCR and CIMD both supported; CIMD chosen only if
  `client_id_metadata_document_supported: true` and `none` in
  `token_endpoint_auth_methods_supported`. With DCR, Claude registers per connection.
- PKCE S256; advertise `code_challenge_methods_supported: ["S256"]`.
- Callbacks `https://claude.ai/api/mcp/auth_callback` and
  `https://claude.com/api/mcp/auth_callback`; Claude Code uses loopback with a random port.
- Token endpoint: form-urlencoded, `invalid_grant` for expired refresh, rotate
  refresh tokens. Timeouts 10 s (30 s refresh). Egress 160.79.104.0/21.

ChatGPT (https://developers.openai.com/plugins/llms-full.txt):

- CIMD preferred (client_id `https://chatgpt.com/oauth/client.json`), DCR works.
- S256, 401 + RFC 9728, echo `resource`.
- With `authorization_response_iss_parameter_supported: true` and an exact `iss`
  in the redirect, the callback is stable:
  `https://chatgpt.com/connector_platform_oauth_redirect`; otherwise
  `https://chatgpt.com/connector/oauth/{callback_id}`.
- No static bearer option.

## Go SDK

`github.com/modelcontextprotocol/go-sdk` v1.8.0: Streamable HTTP, prompts,
resources, `auth.RequireBearerToken` (custom verifier, `TokenInfo` in context),
`ProtectedResourceMetadataHandler`. Known bugs: #1134 (no RFC 6750 error code),
#1349 (403 vs 401 for expired+scope). Verify 401 behaviour in tests.

## Images

ChatGPT can pass uploads via `_meta["openai/fileParams"]`; claude.ai cannot. The
agent transcribes the photo and sends text. Out of scope for MVP.

## Gap-fill authoring

Sources: British Council test question types; Bundled Gap Filling (ACL W16-0519);
CLiC-it 2024 grammar cloze; Nakata & Suzuki 2019 (interleaving, MLJ);
MORFES benchmark (LLM Greek morphology); elon.io Greek grammar pages.

- One target per blank; key unique or all valid answers listed; context forces the
  answer (time adverbs, subject, preposition).
- Hints: lemma or feature tag, never the inflected answer; consistent format.
- 6–14 words, one or two clauses, A2–B1 vocabulary.
- Meaningful sentences, varied frames and blank positions; vary person, number,
  gender, lexical class, polarity, questions.
- Blocked then interleaved (~30 + ~70); irregular forms recur spaced out.
- LLM failure modes: level drift, invented inflections on rare lemmas, ambiguity,
  template repetition, translationese.
- Self-check: batches of 20, blind re-solve, derive from paradigm, native-reading
  pass, mechanical lint (NFC, no polytonic, no Latin lookalikes, final ς, one tonos).
- Greek: accent shift (άνθρωπος → ανθρώπων, έγραψα/γράψαμε), enclitic double accent
  (δάσκαλός μου), monotonic only, aspect after θα/να is the main ambiguity (force
  with κάθε μέρα / αύριο / hint), clitic placement, τη(ν)/δε(ν)/μη(ν), doublets
  ήρθα/ήλθα, αγαπούσα/αγάπαγα.
- Copyright: extract the spec (target, slots, word pool, hint format), write new
  sentences; store only a short source reference.

## Connect deep links

- Claude: `https://claude.ai/customize/connectors?modal=add-custom-connector&connectorName=<name>&connectorUrl=<url>`
  opens the add-custom-connector dialog pre-filled; the user still confirms,
  logs in and allows. https://claude.com/docs/connectors/custom/add-unlisted
- Cursor: `cursor://anysphere.cursor-deeplink/mcp/install?name=<name>&config=<base64 JSON>`,
  where the JSON is `{"url": "<url>"}` for a remote server (or `{"command": ...}`
  for stdio). https://cursor.com/docs/mcp/install-links
- VS Code: `vscode:mcp/install?<url-encoded JSON>`, where the JSON is
  `{"name": "<name>", "type": "http", "url": "<url>"}`; installed servers only
  expose tools to Copilot's agent mode. https://code.visualstudio.com/docs/agent-customization/mcp-servers
- ChatGPT: no prefill or deep-link URL scheme exists for adding a custom MCP
  connector; the user must go through Settings → Apps & Connectors (or
  Plugins) manually, and developer mode gating applies on Plus/Pro.
  https://help.openai.com/en/articles/12584461-developer-mode-and-mcp-apps-in-chatgpt
