package runtimemanager

import (
	"fmt"
	"strings"
)

const defaultRuntimeStartCommand = "/usr/local/bin/start-vmdocker-agent.sh"

func runtimeStartCommandOrDefault(startCommand string) string {
	if strings.TrimSpace(startCommand) == "" {
		return defaultRuntimeStartCommand
	}
	return startCommand
}

func buildForegroundRuntimeCommand(startCommand string) ([]string, error) {
	return parseCommandLine(runtimeStartCommandOrDefault(startCommand))
}

func buildBackgroundRuntimeCommand(startCommand string) (string, error) {
	args, err := parseCommandLine(runtimeStartCommandOrDefault(startCommand))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("mkdir -p \"${TMPDIR:-/tmp}\" && %s >\"${TMPDIR:-/tmp}/vmdocker-agent.log\" 2>&1 &", shellEscapeCommand(args)), nil
}

func parseCommandLine(command string) ([]string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, fmt.Errorf("start command is empty")
	}

	var args []string
	var current strings.Builder
	var quote rune
	escaped := false

	flush := func() {
		if current.Len() == 0 {
			return
		}
		args = append(args, current.String())
		current.Reset()
	}

	for _, r := range command {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case r == ' ' || r == '\t' || r == '\n':
			flush()
		default:
			current.WriteRune(r)
		}
	}

	if escaped {
		return nil, fmt.Errorf("unterminated escape sequence in start command")
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quoted string in start command")
	}

	flush()
	if len(args) == 0 {
		return nil, fmt.Errorf("start command is empty")
	}
	return args, nil
}

func shellEscapeCommand(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellEscapeArg(arg))
	}
	return strings.Join(quoted, " ")
}

func shellEscapeArg(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
