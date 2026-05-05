package dto

type UserRef struct {
	UDID        string `json:"udid"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url,omitempty"`
}
