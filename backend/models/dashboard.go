package models

// Dashboard is the owner's dashboard: the boards (tabs) and the cards on each, with the size of every card. It holds no
// data from any stack, only which cards are placed where and how large. An empty list of boards means "the shipped
// defaults": the screen then builds Home and the stack boards itself, and nothing is stored until the owner changes one.
type Dashboard struct {
	Boards []DashboardBoard `json:"boards"`
}

// DashboardBoard is one tab of the dashboard.
type DashboardBoard struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Cards []DashboardCard `json:"cards"`
}

// DashboardCard is one card on a board, in order. Cols is its width in a grid of twelve columns and Rows its height in rows.
// ID names a card the screen knows; the server does not know which ids exist, so an id it has no card for is skipped on screen.
type DashboardCard struct {
	ID   string `json:"id"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}
