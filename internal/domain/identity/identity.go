// Package identity models tenants, users and what they are allowed to do.
package identity

type Role string

const (
	RoleViewer   Role = "viewer"
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
)

var roleRank = map[Role]int{RoleViewer: 1, RoleOperator: 2, RoleAdmin: 3}

func (r Role) Valid() bool { return roleRank[r] > 0 }

// Can reports whether r has at least the privileges of required.
func (r Role) Can(required Role) bool {
	return r.Valid() && roleRank[r] >= roleRank[required]
}

type Tenant struct {
	ID   int64  `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type User struct {
	ID           int64  `json:"id"`
	TenantID     int64  `json:"tenant_id"`
	TenantSlug   string `json:"tenant"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	Role         Role   `json:"role"`
	PasswordHash string `json:"-"`
}

// Principal is the authenticated caller of a request. Every use case receives
// one, and every query is scoped by its TenantID: tenant isolation is enforced
// in the application layer, not left to each handler.
type Principal struct {
	UserID     int64  `json:"uid"`
	Email      string `json:"email"`
	Name       string `json:"name"`
	TenantID   int64  `json:"tid"`
	TenantSlug string `json:"tenant"`
	Role       Role   `json:"role"`
}

func (u User) Principal() Principal {
	return Principal{
		UserID: u.ID, Email: u.Email, Name: u.Name,
		TenantID: u.TenantID, TenantSlug: u.TenantSlug, Role: u.Role,
	}
}
