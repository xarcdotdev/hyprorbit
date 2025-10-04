package orbit

// Record represents an orbit configuration with display metadata.
type Record struct {
	Name  string
	Label string
	Color string
}

// Summary describes an orbit along with runtime state used by clients.
type Summary struct {
	Name         string `json:"name"`
	Status       string `json:"status"`
	ActiveModule string `json:"active_module,omitempty"`
}
