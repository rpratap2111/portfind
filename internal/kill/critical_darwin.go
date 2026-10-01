package kill

// criticalProcesses are never killed, whatever the user confirms (possible
// with sudo). Killing launchd or kernel_task brings the system down, killing
// WindowServer or loginwindow ends your login session and every app in it,
// killing sshd can lock you out of a remote machine, and killing
// mDNSResponder breaks DNS; stop services with launchctl instead.
var criticalProcesses = []string{"launchd", "kernel_task", "WindowServer", "loginwindow", "sshd", "mDNSResponder"}

// pidZeroReason explains PID 0 in refusal messages: the inspector reports
// sockets whose owner it can't see (another user's process) as PID 0.
const pidZeroReason = "portfind can't see which process owns this port (it belongs to another user; run portfind with sudo)"
