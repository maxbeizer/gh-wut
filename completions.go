package main

import (
	"fmt"
	"os"
)

// GenerateBashCompletion outputs bash completion script
func GenerateBashCompletion() string {
	return `_gh_wut() {
    local cur="${COMP_WORDS[COMP_CWORD]}"
    local commands="context ctx catch-up catchup cu story st dashboard dash db help version"
    COMPREPLY=($(compgen -W "$commands" -- "$cur"))
}
complete -F _gh_wut "gh-wut"
# For use as gh extension:
# eval "$(gh wut --completions bash)"
`
}

// GenerateZshCompletion outputs zsh completion script
func GenerateZshCompletion() string {
	return `#compdef gh-wut
_gh_wut() {
    local -a commands
    commands=(
        'context:Where was I? Open PRs, issues, recent commits'
        'ctx:Alias for context'
        'catch-up:What happened? Notification triage'
        'catchup:Alias for catch-up'
        'cu:Alias for catch-up'
        'story:Full PR story — issue, related PRs, CI'
        'st:Alias for story'
        'dashboard:Cross-repo: everything with your name on it'
        'dash:Alias for dashboard'
        'db:Alias for dashboard'
    )
    _describe 'command' commands
}
_gh_wut "$@"
`
}

// GenerateFishCompletion outputs fish completion script
func GenerateFishCompletion() string {
	return `complete -c gh-wut -f
complete -c gh-wut -n '__fish_use_subcommand' -a context -d 'Where was I?'
complete -c gh-wut -n '__fish_use_subcommand' -a ctx -d 'Alias for context'
complete -c gh-wut -n '__fish_use_subcommand' -a catch-up -d 'What happened?'
complete -c gh-wut -n '__fish_use_subcommand' -a catchup -d 'Alias for catch-up'
complete -c gh-wut -n '__fish_use_subcommand' -a cu -d 'Alias for catch-up'
complete -c gh-wut -n '__fish_use_subcommand' -a story -d 'Full PR story'
complete -c gh-wut -n '__fish_use_subcommand' -a st -d 'Alias for story'
complete -c gh-wut -n '__fish_use_subcommand' -a dashboard -d 'Cross-repo overview'
complete -c gh-wut -n '__fish_use_subcommand' -a dash -d 'Alias for dashboard'
complete -c gh-wut -n '__fish_use_subcommand' -a db -d 'Alias for dashboard'
`
}

func printCompletions(shell string) {
	switch shell {
	case "bash":
		fmt.Print(GenerateBashCompletion())
	case "zsh":
		fmt.Print(GenerateZshCompletion())
	case "fish":
		fmt.Print(GenerateFishCompletion())
	default:
		fmt.Fprintf(os.Stderr, "Unknown shell: %s (supported: bash, zsh, fish)\n", shell)
		os.Exit(1)
	}
}
