package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode"
)

func printCompletionScript(w io.Writer, shell, bin string) error {
	cmd := filepath.Base(bin)
	name := sanitizeShellIdent(cmd)
	switch shell {
	case "bash":
		fmt.Fprintf(w, bashCompletionTemplate, name, bin, name, bin)
	case "zsh":
		fmt.Fprintf(w, zshCompletionTemplate, bin, name, bin, name, bin)
	case "fish":
		fmt.Fprintf(w, fishCompletionTemplate, name, bin, cmd, name)
	}
	return nil
}

// sanitizeShellIdent turns a binary name into a valid shell identifier.
func sanitizeShellIdent(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsLetter(r) || r == '_' || (i > 0 && unicode.IsDigit(r)) {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

// bashCompletionTemplate aligns with the official go-flags bash completion
// example, with one addition: when the current token is empty we inject "--"
// so go-flags completes option names rather than falling through to positional
// completion (our positional args are plain strings with no Completer).
var bashCompletionTemplate = `_%s() {
    local args=("${COMP_WORDS[@]:1:$COMP_CWORD}")
    if [ ${#args[@]} -eq 0 ] || ([ ${#args[@]} -eq 1 ] && [ -z "${args[0]}" ]); then
        args=("--")
    fi
    local IFS=$'\n'
    COMPREPLY=($(GO_FLAGS_COMPLETION=1 %s "${args[@]}"))
    return 0
}
complete -F _%s %s
`

// zshCompletionTemplate aligns with the open PR #414 for go-flags zsh
// completion, with the same empty-token "--" injection.
var zshCompletionTemplate = `#compdef %s
_%s() {
    local args=("${words[@]:1}")
    if [ ${#args[@]} -eq 0 ] || ([ ${#args[@]} -eq 1 ] && [ -z "${args[1]}" ]); then
        args=("--")
    fi
    local IFS=$'\n'
    local -a completions
    completions=($(GO_FLAGS_COMPLETION=1 %s "${args[@]}"))
    compadd -a completions
}
compdef _%s %s
`

// fishCompletionTemplate follows fish conventions (commandline -opc / -ct)
// with the same empty-token "--" injection and explicit "" append for value
// completion.
var fishCompletionTemplate = `function __%s_complete
    set -l args (commandline -opc | tail -n +2)
    set -l cur (commandline -ct)
    if test (count $args) -eq 0
        set args "--"
    else if test -z "$cur"
        set args $args ""
    end
    env GO_FLAGS_COMPLETION=1 %s $args
end
complete -c %s -f -a '(__%s_complete)'
`
