# jj mistranslates `HEAD^!` and approximates `HEAD^N`

`(*Jj).translateRef` in `app/diff/jj.go` has a broad `strings.HasPrefix(ref, "HEAD^")` branch that
swallows two different inputs and answers both with `parents(@-)`:

- `HEAD^!` is git revision-set syntax jj does not support. It becomes `parents(@-)`, and
  `diffRangeFlags` then builds `--from parents(@-) --to @`, so the user gets a grandparent-to-working-copy
  diff that looks plausible and carries no error, while git rejects `HEAD^!..HEAD` for the commit log
  and hg emits the invalid revset `p!(.)`, so jj is the only backend that fails silently here.
- `HEAD^N` for N greater than 1 means the Nth parent. The branch's own comment admits jj cannot single
  out an individual parent in one revset step and calls `parents(@-)` a best-effort approximation.

Found while investigating #335 (closed as not planned); not part of that issue and not caused by it.
hg's equivalent needs no change: its `pN(.)` mapping is correct.

Not pre-deciding the fix. Two options, and they can differ per input: fail closed on a malformed
suffix so the user sees a rejection instead of wrong data, or implement exact Nth-parent selection if
jj can express it. Rejecting `HEAD^N` is a behavior change beyond a bug fix, so it needs a decision.

A regression must assert at the public operation, e.g. `ChangedFiles("HEAD^!", false)` returning an
error against a real jj repo. Asserting that `translateRef` returns the raw string does not prove the
user ever sees the rejection.
