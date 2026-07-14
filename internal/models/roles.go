package models

// Role values stored on users.role. Kept as plain strings (not a DB enum)
// so adding a role later is a migration-free change.
const (
	RolePrincipal = "principal"
	RoleTeacher   = "teacher"
	RoleParent    = "parent"
)
