---
worth: maybe
where: app/revdiff/config.go:parseArgs
added: 2026-10-05
---
# a config file value overrides the matching env var, against the documented precedence

The documented order is `CLI flags > env vars > config file > built-in defaults`: the comment on
`parseArgs`, `site/docs.html`, and `references/config.md` in the plugin skills all say so. The code
does `CLI > config file > env`. With `REVDIFF_WRAP=true` and `REVDIFF_TAB_WIDTH=8` set and a config
file holding `wrap = false` and `tab-width = 2`, `parseArgs` returns `Wrap=false`, `TabWidth=2`.

Cause: `parseArgs` loads the INI file before `ParseArgs`, and go-flags' INI parser calls `Set` on each
option it reads. `Set` marks the option as explicitly set, so the env default that `ParseArgs` would
apply afterwards is skipped. Only an option absent from the config file still takes its env var.

`maybe` because the choice is not made. Correcting the code changes behavior for anyone who has both
an env var and a config key for the same option and relies on the file winning; correcting the docs
keeps behavior and admits an order nobody chose. The code fix is not worked out. Parsing args first
and loading the INI with `ParseAsDefaults` protects CLI flags only: an env value is applied through
`setDefault`, which leaves `preventDefault` false, so the INI still overwrites it. Env values would
have to be preserved explicitly, with `--config` resolution and `--dump-config` kept working. No test
pins either order today: the env and config-file cases for each flag are separate subtests. Surfaced
while finishing #371.
