package dto

// Notification is one bell entry. Title and Body are rendered server-side so the
// wording lives in one place and phase 2's messengers reuse it verbatim.
type Notification struct {
	ID          int64  `json:"id"`
	Type        string `json:"type"`
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	Count       int    `json:"count"`
	CreatedAt   string `json:"created_at"`
	Read        bool   `json:"read"`
	ActorName   string `json:"actor_name"`
	ActorAvatar string `json:"actor_avatar,omitempty"`
	// URL is where clicking the notification navigates. Empty when the target is gone.
	URL string `json:"url,omitempty"`
}

type NotificationList struct {
	Items      []Notification `json:"items"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

type UnreadCount struct {
	Count int `json:"count"`
}

// NotificationPreference is one row of the settings matrix.
// Scope is empty for addressed types, where it does not apply.
type NotificationPreference struct {
	Type     string   `json:"type"`
	Enabled  bool     `json:"enabled"`
	Scope    string   `json:"scope,omitempty"`
	Channels []string `json:"channels"`
	// Addressed marks a type that has no scope selector, so the UI renders a dash
	// instead of a dropdown without hardcoding the type name.
	Addressed bool `json:"addressed"`
}

type NotificationPreferences struct {
	Items []NotificationPreference `json:"items"`
	// Channels available in this tenant. Phase 1b always returns ["in_app"]; the UI
	// shows channel columns only when there is more than one.
	Channels []string `json:"channels"`
}
