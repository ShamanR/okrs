package dto

// Notification is one bell entry. Title and Body are rendered server-side so the
// wording lives in one place and phase 2's messengers reuse it verbatim.
type Notification struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Body  string `json:"body"`
	// Subject names what the notification is about, shown as its own line in the
	// card. Sent only when the body does not already name the entity — today that
	// is the key-result check-in alone (internal/render/notify decides, not the
	// client). Omitted when empty so the client renders nothing rather than an
	// empty line.
	Subject     string `json:"subject,omitempty"`
	Count       int    `json:"count"`
	CreatedAt   string `json:"created_at"`
	Read        bool   `json:"read"`
	ActorName   string `json:"actor_name"`
	ActorAvatar string `json:"actor_avatar,omitempty"`
	// URL is where clicking the notification navigates. Empty when the target is gone.
	URL string `json:"url,omitempty"`
	// Context is where this happened, so a long list can be scanned without opening
	// entries. Resolved on read, not frozen at write time: a renamed team or goal
	// shows its current name, which is what makes the list navigable. Omitted whole
	// when the notification has neither.
	Context *NotificationContext `json:"context,omitempty"`
}

// NotificationContext is the notification's place in the org: the team with its
// ancestors, root first, and the goal it happened on.
type NotificationContext struct {
	// Team is the path from the root, joined by " / " — "Компания / Платформа".
	Team string `json:"team,omitempty"`
	// Goal is the goal's title. Empty when the notification has no goal, or the
	// goal was deleted after the notification was written.
	Goal string `json:"goal,omitempty"`
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
	Type    string `json:"type"`
	Enabled bool   `json:"enabled"`
	Scope   string `json:"scope,omitempty"`
	// Channels is this row's state per channel: every channel the tenant delivers
	// to, mapped to whether this user gets this type there. Effective state, not
	// stored state — a channel the user never chose about reports the
	// administrator's default, which is what the checkbox has to show.
	Channels map[string]bool `json:"channels"`
	// Addressed marks a type that has no scope selector, so the UI renders a dash
	// instead of a dropdown without hardcoding the type name.
	Addressed bool `json:"addressed"`
}

// NotificationChannelOption is one column of the settings matrix. Title comes
// from the channel's own descriptor, so a channel added by another module labels
// itself and the screen needs no knowledge of it.
type NotificationChannelOption struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	// DefaultOn is what the administrator chose for staff who never decided. The
	// UI needs it to offer "back to default" and to explain an unchecked column.
	DefaultOn bool `json:"default_on"`
}

type NotificationPreferences struct {
	Items []NotificationPreference `json:"items"`
	// Channels are the columns of the matrix, the bell first and external
	// channels in build order.
	Channels []NotificationChannelOption `json:"channels"`
}
