package kill

// criticalProcesses are never killed, whatever the user confirms. Terminating
// any of these (possible when elevated) crashes or reboots Windows; svchost
// instances host core services such as RPC and should be stopped through the
// Services manager instead.
var criticalProcesses = []string{
	"system", "system idle process", "registry", "smss", "csrss",
	"wininit", "winlogon", "services", "lsass", "svchost",
}

// pidZeroReason explains PID 0 in refusal messages.
const pidZeroReason = "it is the System Idle Process"
