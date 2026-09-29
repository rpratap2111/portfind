package kill

// criticalProcesses are never killed, whatever the user confirms (possible
// with sudo). Killing init brings the system down, killing sshd can lock you
// out of a remote machine, and killing systemd-resolved breaks DNS; stop
// services with systemctl instead.
var criticalProcesses = []string{"systemd", "init", "sshd", "systemd-resolved"}

// pidZeroReason explains PID 0 in refusal messages: the inspector reports
// sockets whose owner it can't see (another user's process) as PID 0.
const pidZeroReason = "portfind can't see which process owns this port (it belongs to another user; run portfind with sudo)"
