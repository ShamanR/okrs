package domain

import "time"

type TenantStatus string

const (
	TenantActive    TenantStatus = "active"
	TenantSuspended TenantStatus = "suspended"
)

type Tenant struct {
	ID        int64
	Slug      string
	Name      string
	Status    TenantStatus
	CreatedAt time.Time
	DeletedAt *time.Time
}

type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

type MembershipStatus string

const (
	MembershipActive    MembershipStatus = "active"
	MembershipRequested MembershipStatus = "requested"
)

type Membership struct {
	ID              int64
	UserID          int64
	TenantID        int64
	Role            Role
	Status          MembershipStatus
	CreatedAt       time.Time
	CreatedByUserID *int64
}

type InvitationStatus string

const (
	InvitationPending InvitationStatus = "pending"
	InvitationClaimed InvitationStatus = "claimed"
	InvitationRevoked InvitationStatus = "revoked"
)

type Invitation struct {
	ID              int64
	TenantID        int64
	Email           string
	Role            Role
	Status          InvitationStatus
	CreatedByUserID *int64
	CreatedAt       time.Time
	ExpiresAt       *time.Time
}

var reservedTenantSlugs = map[string]bool{
	"www": true, "api": true, "app": true, "admin": true, "static": true,
	"assets": true, "mail": true, "auth": true, "system": true,
}

// ValidTenantSlug enforces the slug grammar ^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$
// (lowercase, 2..32 chars, no leading/trailing dash) and rejects reserved subdomains.
func ValidTenantSlug(s string) bool {
	if len(s) < 2 || len(s) > 32 || reservedTenantSlugs[s] {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		isLower := c >= 'a' && c <= 'z'
		isDigit := c >= '0' && c <= '9'
		isDash := c == '-'
		if !isLower && !isDigit && !isDash {
			return false
		}
		if isDash && (i == 0 || i == len(s)-1) {
			return false
		}
	}
	return true
}
