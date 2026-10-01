package commands

// controlCommands are commands that change how the running agent is
// configured rather than the conversation itself: /reload re-reads the
// config file and /switch replaces the model. Channels whose input is not
// fully trusted (the pico WebSocket forwards text a remote client, and
// indirectly a model reading untrusted content, can control) use
// IsControlCommand to refuse them unless explicitly allowed.
var controlCommands = map[string]struct{}{
	"reload": {},
	"switch": {},
}

// IsControlCommand reports whether input invokes a control command,
// using the same prefix and name parsing as the command executor.
func IsControlCommand(input string) bool {
	name, ok := parseCommandName(input)
	if !ok {
		return false
	}
	_, found := controlCommands[name]
	return found
}
