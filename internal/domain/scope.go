package domain

// TenantScope is the per-request tenant boundary passed explicitly into scoped
// repository methods. Carrying it as a named type (not a bare int64) prevents
// accidentally passing some other id as the tenant.
type TenantScope struct {
	TenantID int64
}
