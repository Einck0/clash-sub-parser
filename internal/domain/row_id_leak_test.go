package domain_test

import (
	"reflect"
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
)

// TestNoDatabaseRowIDLeak verifies that domain models NEVER expose internal
// database auto-incrementing integer IDs or leak row IDs.
// All domain public IDs must be UUIDv7 strings or stable logical IDs.
func TestNoDatabaseRowIDLeak(t *testing.T) {
	entities := []interface{}{
		domain.Subscription{},
		domain.SubscriptionFetch{},
		domain.Node{},
		domain.NodeSource{},
		domain.ProbeRun{},
		domain.ProbeObservation{},
		domain.NodeGroup{},
		domain.GroupEdge{},
		domain.AdmissionRule{},
		domain.PolicyRule{},
		domain.ConfigurationRevision{},
		domain.Publication{},
		domain.Settings{},
		domain.AuditEvent{},
	}

	for _, entity := range entities {
		typ := reflect.TypeOf(entity)
		t.Run(typ.Name(), func(t *testing.T) {
			for i := 0; i < typ.NumField(); i++ {
				field := typ.Field(i)
				fieldNameLower := strings.ToLower(field.Name)

				// Check 1: No field named row_id, db_id, auto_inc, etc.
				forbiddenNames := []string{"rowid", "row_id", "dbid", "db_id", "autoid", "auto_id", "autoinc"}
				for _, forbidden := range forbiddenNames {
					if strings.Contains(fieldNameLower, forbidden) {
						t.Errorf("entity %s contains forbidden database row ID field name: %s", typ.Name(), field.Name)
					}
				}

				// Check 2: If field is an identifier (e.g. ID, SubscriptionID, ParentGroupID, etc.),
				// it MUST NOT be an integer!
				if field.Name == "ID" || strings.HasSuffix(field.Name, "ID") || strings.HasSuffix(field.Name, "Id") {
					kind := field.Type.Kind()
					if kind == reflect.Int || kind == reflect.Int32 || kind == reflect.Int64 ||
						kind == reflect.Uint || kind == reflect.Uint32 || kind == reflect.Uint64 {
						t.Errorf("entity %s field %s is an integer identifier (%v) - leaky database row ID forbidden in domain!",
							typ.Name(), field.Name, kind)
					}
				}
			}
		})
	}
}
