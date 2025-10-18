package presenter

// WindowMoveResult captures the outcome of moving a window.
type WindowMoveResult struct {
	Window    string `json:"window"`
	Workspace string `json:"workspace"`
	Module    string `json:"module,omitempty"`
	Orbit     string `json:"orbit,omitempty"`
	Created   bool   `json:"created,omitempty"`
	Temporary bool   `json:"temporary,omitempty"`
	Focused   bool   `json:"focused"`
}

// WindowSummary captures metadata about a Hyprland window relevant to move operations.
type WindowSummary struct {
	Address   string `json:"address"`
	Class     string `json:"class,omitempty"`
	Title     string `json:"title,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Module    string `json:"module,omitempty"`
	Orbit     string `json:"orbit,omitempty"`
}
