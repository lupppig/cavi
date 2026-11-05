package authz

import "testing"

func TestRolesAreValid(t *testing.T) {
	for _, r := range Roles {
		if !ValidRole(r) {
			t.Errorf("role %q not valid", r)
		}
	}
	for _, r := range []string{"", "superuser", "Owner"} {
		if ValidRole(r) {
			t.Errorf("role %q should be invalid", r)
		}
	}
}

func TestPermissionMatrix(t *testing.T) {
	cases := []struct {
		role string
		p    Permission
		want bool
	}{
		{RoleOwner, BillingManage, true},
		{RoleAdmin, BillingManage, false},
		{RoleAdmin, OrganisationMembersManage, true},
		{RoleManager, OrganisationMembersManage, false},
		{RoleManager, PublishingManage, true},
		{RoleEditor, ProjectDelete, false},
		{RoleEditor, EditingManage, true},
		{RoleCreator, ApprovalReview, false},
		{RoleReviewer, ApprovalReview, true},
		{RoleReviewer, ProjectUpdate, false},
		{RoleViewer, ProjectRead, true},
		{RoleViewer, MediaUpload, false},
		{RoleClient, OrganisationMembersRead, false},
		{RoleClient, ApprovalReview, true},
		{RoleManager, ProjectAccessManage, true},
		{RoleEditor, ProjectAccessManage, false},
		{RoleClient, ProjectAccessManage, false},
		{"unknown", ProjectRead, false},
	}
	for _, tc := range cases {
		if got := Can(tc.role, tc.p); got != tc.want {
			t.Errorf("Can(%s, %s) = %v, want %v", tc.role, tc.p, got, tc.want)
		}
	}
}

func TestOwnerHasEveryPermission(t *testing.T) {
	if len(PermissionsFor(RoleOwner)) != len(all) {
		t.Errorf("owner has %d of %d permissions", len(PermissionsFor(RoleOwner)), len(all))
	}
	if len(PermissionsFor("unknown")) != 0 {
		t.Error("unknown role has permissions")
	}
}
