// Package authz defines CAVI's roles and the permissions each one grants.
//
// Permissions describe capabilities; roles are bundles of them. Every
// authorisation decision, in HTTP handlers and background workers alike,
// goes through Can.
package authz

import "slices"

// Permission is a capability within an organisation.
type Permission string

// Permissions.
const (
	OrganisationManage        Permission = "organisation.manage"
	OrganisationMembersRead   Permission = "organisation.members.read"
	OrganisationMembersManage Permission = "organisation.members.manage"
	WorkspaceCreate           Permission = "workspace.create"
	WorkspaceManage           Permission = "workspace.manage"
	ProjectCreate             Permission = "project.create"
	ProjectRead               Permission = "project.read"
	ProjectUpdate             Permission = "project.update"
	ProjectDelete             Permission = "project.delete"
	MediaUpload               Permission = "media.upload"
	MediaDelete               Permission = "media.delete"
	EditingManage             Permission = "editing.manage"
	RenderExecute             Permission = "render.execute"
	PublishingManage          Permission = "publishing.manage"
	BrandManage               Permission = "brand.manage"
	BillingManage             Permission = "billing.manage"
	ApprovalReview            Permission = "approval.review"
)

// Role names, from most to least privileged.
const (
	RoleOwner    = "owner"
	RoleAdmin    = "admin"
	RoleManager  = "manager"
	RoleEditor   = "editor"
	RoleCreator  = "creator"
	RoleReviewer = "reviewer"
	RoleViewer   = "viewer"
	RoleClient   = "client"
)

// Roles lists every role, most privileged first.
var Roles = []string{RoleOwner, RoleAdmin, RoleManager, RoleEditor, RoleCreator, RoleReviewer, RoleViewer, RoleClient}

var all = []Permission{
	OrganisationManage, OrganisationMembersRead, OrganisationMembersManage,
	WorkspaceCreate, WorkspaceManage,
	ProjectCreate, ProjectRead, ProjectUpdate, ProjectDelete,
	MediaUpload, MediaDelete, EditingManage, RenderExecute,
	PublishingManage, BrandManage, BillingManage, ApprovalReview,
}

var grants = map[string][]Permission{
	RoleOwner: all,
	RoleAdmin: without(all, BillingManage),
	RoleManager: {
		OrganisationMembersRead, WorkspaceCreate, WorkspaceManage,
		ProjectCreate, ProjectRead, ProjectUpdate, ProjectDelete,
		MediaUpload, MediaDelete, EditingManage, RenderExecute,
		PublishingManage, BrandManage, ApprovalReview,
	},
	RoleEditor: {
		OrganisationMembersRead, ProjectCreate, ProjectRead, ProjectUpdate,
		MediaUpload, EditingManage, RenderExecute, ApprovalReview,
	},
	RoleCreator: {
		OrganisationMembersRead, ProjectCreate, ProjectRead, ProjectUpdate,
		MediaUpload, EditingManage, RenderExecute,
	},
	RoleReviewer: {OrganisationMembersRead, ProjectRead, ApprovalReview},
	RoleViewer:   {OrganisationMembersRead, ProjectRead},
	// Clients are external: they see only what they are explicitly given
	// and cannot list the organisation's members.
	RoleClient: {ProjectRead, ApprovalReview},
}

// ValidRole reports whether role is a known role.
func ValidRole(role string) bool {
	_, ok := grants[role]
	return ok
}

// Can reports whether role grants p. Unknown roles grant nothing.
func Can(role string, p Permission) bool {
	return slices.Contains(grants[role], p)
}

// PermissionsFor returns the permissions role grants, in a stable order.
func PermissionsFor(role string) []Permission {
	out := make([]Permission, 0, len(grants[role]))
	for _, p := range all {
		if Can(role, p) {
			out = append(out, p)
		}
	}
	return out
}

func without(ps []Permission, drop Permission) []Permission {
	out := make([]Permission, 0, len(ps))
	for _, p := range ps {
		if p != drop {
			out = append(out, p)
		}
	}
	return out
}
