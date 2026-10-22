package identity

import "testing"

func TestRoleCan(t *testing.T) {
	tests := []struct {
		role, required Role
		want           bool
	}{
		{RoleAdmin, RoleOperator, true},
		{RoleOperator, RoleOperator, true},
		{RoleViewer, RoleOperator, false},
		{Role(""), RoleViewer, false},
		{Role("root"), RoleViewer, false},
	}
	for _, tt := range tests {
		if got := tt.role.Can(tt.required); got != tt.want {
			t.Errorf("%q.Can(%q) = %v, want %v", tt.role, tt.required, got, tt.want)
		}
	}
}
