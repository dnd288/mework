package registry

import "testing"

// TestTenant_FieldsRoundTrip asserts the Tenant primitive exists with ID and Name
// fields and that values round-trip. This realizes the delta-spec scenario
// "Operator registers a tenant" at the type level: a tenant has a stable identifier
// and a name.
func TestTenant_FieldsRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		tenantNm string
	}{
		{name: "acme", id: "t1", tenantNm: "acme"},
		{name: "globex", id: "t2", tenantNm: "globex"},
		{name: "empty name", id: "t3", tenantNm: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ten := Tenant{ID: tt.id, Name: tt.tenantNm}
			if ten.ID != tt.id {
				t.Errorf("Tenant.ID = %q, want %q", ten.ID, tt.id)
			}
			if ten.Name != tt.tenantNm {
				t.Errorf("Tenant.Name = %q, want %q", ten.Name, tt.tenantNm)
			}
		})
	}
}

// TestRuntime_CarriesTenantID asserts every tenant-scoped struct in the package
// (here, Runtime) carries a TenantID so resources can be keyed by their tenant.
// This realizes the delta-spec requirement "Tenants are isolated from each other":
// every tenant-scoped resource MUST be keyed by its tenant.
func TestRuntime_CarriesTenantID(t *testing.T) {
	tests := []struct {
		name     string
		tenantID string
	}{
		{name: "acme tenant", tenantID: "t1"},
		{name: "globex tenant", tenantID: "t2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := Runtime{TenantID: tt.tenantID}
			if rt.TenantID != tt.tenantID {
				t.Errorf("Runtime.TenantID = %q, want %q", rt.TenantID, tt.tenantID)
			}
		})
	}
}
