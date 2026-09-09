# Cache and offline status

There is currently **no reusable Zap machine cache**. `ZAP_CACHE_DIR`,
`zap cache status/path/clean` and `zap sync --offline` are planned interfaces,
not supported features. Git sources live in project-local dependency checkouts.

`zap verify --offline` skips remote lookups and retains lock compatibility and
local Git checks. `zap make --offline` uses those checks before configuring and
building already-present Git/path dependencies. Missing managed Git source is
an error; run online `zap sync` to restore it. URL dependencies are rejected in
offline verification because Zap cannot verify their materialised content.

`-DZAP_OFFLINE=ON` requests local verification in generated CMake and prevents
its missing-Git-source fetch fallback. It does not stop arbitrary project CMake,
SDKs or build tools from making their own network requests.

Git status can miss hidden edits and ignored files. See [SECURITY.md](../SECURITY.md)
before relying on these checks.

Planned cache work includes independent source hashes, safe extraction,
verification before and after materialisation, atomic installation and restoration
without the remote. None of these cache guarantees is offered today.
