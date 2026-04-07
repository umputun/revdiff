# shell-quote a single argument for safe embedding in sh -c strings.
# wraps in single quotes, escaping embedded quotes via the '\'' idiom.
# usage: sq "value" → produces a POSIX-safe quoted string
sq() { printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"; }
