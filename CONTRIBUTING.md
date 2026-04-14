# Contributing to revdiff

Thank you for your interest in contributing!

## Before You Start

### First-time contributors

If this is your first PR to revdiff, please **open an issue first** describing what you plan to build and why. Wait for maintainer approval before writing code. This saves everyone's time — some ideas may not fit the project's direction, and it's better to find out early than after investing effort into a PR.

### Check existing functionality

Before suggesting a feature or filing an issue, make sure the functionality doesn't already exist. revdiff may already support what you need through existing flags, keybindings, config options, or themes. Run `revdiff --help`, check the [docs](https://revdiff.com/docs.html), and try the feature before proposing new code.

### Ideas and general questions

Use [Discussions](https://github.com/umputun/revdiff/discussions), not Issues, for general questions, ideas, and brainstorming. Issues are for concrete, well-defined bugs or approved feature requests.

### Is it worth it?

Before submitting a PR, critically evaluate the tradeoff between what the feature adds and the code it introduces. A minor or edge-case improvement that inflates the codebase significantly is usually not a good tradeoff. Consider:

- **Does it benefit most users or just a niche case?** Features that affect a handful of edge scenarios rarely justify the maintenance cost.
- **Does it belong in revdiff or upstream?** If a library (chroma, bubbletea, etc.) is missing something, contribute there first. Hardcoding workarounds in revdiff sets a bad precedent.
- **Does it change the tool's scope?** revdiff is a diff/file review TUI. PRs that expand it into something else (staging tool, file manager, etc.) will be rejected.
- **Is the code proportional to the value?** A 500-line PR for a feature used by 1% of users is a hard sell. Keep it simple, keep it small.

## Development Setup

1. Fork the repository
2. Clone your fork: `git clone https://github.com/your-username/revdiff.git`
3. Create a feature branch: `git checkout -b feature-name`
4. Make your changes
5. Run tests: `make test`
6. Run linter: `make lint`
7. Format code: `make fmt`
8. Commit your changes: `git commit -am 'Add feature'`
9. Push to the branch: `git push origin feature-name`
10. Submit a pull request

## Code Style

Please follow the code style guidelines in [CLAUDE.md](CLAUDE.md).

## Issues and PRs

Every issue and PR must clearly describe:

1. **What is the problem?** — what exactly is broken, missing, or inconvenient? Be specific. "It would be nice to have X" is not a problem statement.
2. **How does this solve it?** — explain why this particular approach is the right fix and how it addresses the root cause.

PRs without a clear problem statement will be closed. If you can't articulate the problem, the solution is probably not needed.

## Pull Request Process

1. Update the README.md with details of changes if applicable
2. The PR should work for all configured platforms and pass all tests
3. PR will be merged once it receives approval from maintainers
