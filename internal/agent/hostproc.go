package agent

// ProcessSnapshot is one row in the Task Manager processes list.
type ProcessSnapshot struct {
	Name  string  `json:"name"`
	PID   uint32  `json:"pid"`
	CPU   float64 `json:"cpu"`
	MemMB float64 `json:"memMb"`
}
