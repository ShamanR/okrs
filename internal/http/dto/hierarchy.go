package dto

type HierarchyResponse struct {
	Items []TeamNode `json:"items"`
}

type TeamNode struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Type      string     `json:"type"`
	TypeLabel string     `json:"type_label"`
	Lead      string     `json:"lead"`
	HasGoals  bool       `json:"has_goals"`
	Progress  *int       `json:"progress,omitempty"`
	Children  []TeamNode `json:"children"`
}
