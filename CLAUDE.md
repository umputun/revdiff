# Development Guidelines

## Build & Test Commands
- Build: `go build ./...` or `make build`
- Run tests: `go test ./...` or `make test`
- Run specific test: `go test -run TestName ./path/to/package`
- Run tests with coverage: `go test -cover ./...`
- Run linting: `golangci-lint run`
- Format code: `~/.claude/format.sh` (runs gofmt, goimports, and unfuck-ai-comments)
- Run code generation: `go generate ./...`
- Coverage report: `go test -race -coverprofile=coverage.out ./... && go tool cover -func=coverage.out`
- On completion, run: formatting, tests, and linter
- Never commit without running completion sequence

## Important Workflow Notes
- Always run tests, linter and normalize comments before committing
- For linter use `golangci-lint run`
- Run tests and linter after making significant changes to verify functionality
- Go version: 1.26+
- Don't add "Generated with Claude Code" or "Co-Authored-By: Claude" to commit messages or PRs
- Do not include "Test plan" sections in PR descriptions
- Do not add comments that describe changes, progress, or historical modifications
- Use `go:generate` for generating mocks, never modify generated files manually. Mocks are generated with `moq` and stored in the `mocks` package.
- After important functionality added, update README.md accordingly

## Code Style Guidelines
- Follow [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Use snake_case for filenames, camelCase for variables, PascalCase for exported names
- Group imports: standard library, blank line, third-party, blank line, local packages
- Error handling: check errors immediately and return them with context
- Return early when possible to avoid deep nesting
- Interfaces: define interfaces in consumer packages
- Code width: keep lines under 130 characters when possible
- Logging: never use fmt.Printf for logging, only log.Printf

### Error Handling
- Use `fmt.Errorf("context: %w", err)` to wrap errors with context
- Check errors immediately after function calls
- Return detailed error information through wrapping

### Comments
- All comments inside functions should be lowercase
- Document all exported items with proper casing (godoc comments start with element name)
- Never add historical comments describing changes, progress, or evolution
- Never use emojis in comments or any code

### Testing
- Use table-driven tests where appropriate
- Use subtest with `t.Run()` to make test more structured
- Use `require` for fatal assertions, `assert` for non-fatal ones
- Use mock interfaces for dependency injection with moq
- Test names follow pattern: `Test<Type>_<method>`
- One test file per source file (foo.go -> foo_test.go)
- Mock generation: `//go:generate moq -out mocks/interface.go -pkg mocks -skip-ensure -fmt goimports . InterfaceName`

## Libraries
- CLI flags: `github.com/jessevdk/go-flags`
- TUI: `github.com/charmbracelet/bubbletea` with `github.com/charmbracelet/lipgloss` and `github.com/charmbracelet/bubbles`
- Testing: `github.com/stretchr/testify`
- Mock generation: `github.com/matryer/moq`

## Project Structure
- `cmd/revdiff/` - main application entry point
- `diff/` - git diff parsing and file rendering
- `annotation/` - annotation storage and output formatting
- `ui/` - bubbletea TUI model and views
