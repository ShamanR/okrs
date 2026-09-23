package notificationprefs

// Notification types.
const (
	TypeGoalComment       = "goal_comment"
	TypeMyCommentResolved = "my_comment_resolved"
	TypeGoalChanged       = "goal_changed"
	TypeKRProgress        = "kr_progress"
	TypeAccessRequested   = "access_requested"
)

// Categories group types on the settings screen.
const (
	CategoryGoals  = "goals"
	CategorySystem = "system"
)

// Audiences say how a type finds its recipients.
const (
	// AudienceTeamTree walks the team tree up from the event's team, per each
	// lead's scope preference.
	AudienceTeamTree = "team_tree"
	// AudienceAddressee is one recipient carried in the event itself.
	AudienceAddressee = "addressee"
	// AudienceTenantAdmins is every active admin of the event's tenant.
	AudienceTenantAdmins = "tenant_admins"
)

// TypeInfo is everything the rest of the system needs to know about a type, kept
// in one place so no layer grows its own `if type == ...`.
type TypeInfo struct {
	Type     string
	Category string
	Audience string
	// AdminOnly types are shown and saved only by a tenant admin: nobody else
	// could ever receive them.
	AdminOnly bool
	// DefaultEnabled is what a user who never touched the type gets. System types
	// are off by default so that shipping one does not mail every admin at once.
	DefaultEnabled bool
}

// Catalog is the closed set of types, in the order the settings screen renders
// them: types of one category are adjacent, goals before system.
var Catalog = []TypeInfo{
	{Type: TypeGoalComment, Category: CategoryGoals, Audience: AudienceTeamTree, DefaultEnabled: true},
	{Type: TypeMyCommentResolved, Category: CategoryGoals, Audience: AudienceAddressee, DefaultEnabled: true},
	{Type: TypeGoalChanged, Category: CategoryGoals, Audience: AudienceTeamTree, DefaultEnabled: true},
	{Type: TypeKRProgress, Category: CategoryGoals, Audience: AudienceTeamTree, DefaultEnabled: true},
	{Type: TypeAccessRequested, Category: CategorySystem, Audience: AudienceTenantAdmins, AdminOnly: true},
}

// AllTypes is every type in catalog order.
var AllTypes = func() []string {
	out := make([]string, len(Catalog))
	for i, ti := range Catalog {
		out[i] = ti.Type
	}
	return out
}()

func info(t string) (TypeInfo, bool) {
	for _, ti := range Catalog {
		if ti.Type == t {
			return ti, true
		}
	}
	return TypeInfo{}, false
}

// IsAddressed reports whether a type has no scope selector: its recipients are
// not found through the team tree.
func IsAddressed(t string) bool {
	ti, ok := info(t)
	return ok && ti.Audience != AudienceTeamTree
}

// AudienceOf returns how a type finds its recipients; "" for an unknown type.
func AudienceOf(t string) string {
	ti, _ := info(t)
	return ti.Audience
}

// CategoryOf returns a type's settings category; "" for an unknown type.
func CategoryOf(t string) string {
	ti, _ := info(t)
	return ti.Category
}

// DefaultEnabled reports whether a type is on for a user who never chose.
func DefaultEnabled(t string) bool {
	ti, _ := info(t)
	return ti.DefaultEnabled
}

// TypesFor lists the types a user may see and save, in catalog order.
func TypesFor(isAdmin bool) []string {
	out := make([]string, 0, len(Catalog))
	for _, ti := range Catalog {
		if ti.AdminOnly && !isAdmin {
			continue
		}
		out = append(out, ti.Type)
	}
	return out
}
