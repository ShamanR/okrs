package dto

// NotificationChannelField describes one input of a channel's configuration form.
// The admin screen renders from this and knows nothing about any specific channel —
// which is what lets a channel from another module get a working form for free.
type NotificationChannelField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Kind     string `json:"kind"` // text | url | secret
	Required bool   `json:"required"`
	Hint     string `json:"hint,omitempty"`
}

// NotificationChannelDTO is one channel as the tenant admin sees it.
//
// There is deliberately no field carrying the secret itself: SecretHint is a mask
// ("••••4821"), enough to tell "a token is saved" from "no token yet" and not enough
// to be one. Sending the plaintext back — even to an admin, even over TLS — would put
// it in browser memory, in devtools, and in anything that proxies the response.
type NotificationChannelDTO struct {
	Name       string                     `json:"name"`
	Title      string                     `json:"title"`
	Enabled    bool                       `json:"enabled"`
	Configured bool                       `json:"configured"`
	SecretHint string                     `json:"secret_hint,omitempty"`
	Values     map[string]any             `json:"values"`
	Fields     []NotificationChannelField `json:"fields"`
}
