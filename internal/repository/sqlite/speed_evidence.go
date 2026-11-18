package sqlite

import (
	"database/sql/driver"
	"time"

	modernsqlite "modernc.org/sqlite"

	"clash-sub-parser/internal/domain"
)

func init() {
	// Register once on the driver, before any pool (including sql.Open in tests).
	modernsqlite.MustRegisterDeterministicScalarFunction("csp_speed_kib_s", 3, speedKiBS)
}

func speedKiBS(_ *modernsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	raw, ok := args[0].([]byte)
	if !ok {
		return nil, nil
	}
	stamp, _ := args[1].(string)
	bound, _ := args[2].(string)
	observed, err := time.Parse(time.RFC3339, stamp)
	if err != nil || observed.IsZero() {
		return nil, nil
	}
	asOf, err := time.Parse(time.RFC3339, bound)
	if err != nil || observed.After(asOf) {
		return nil, nil
	}
	ed, ok := domain.DecodeObservationEvidence(raw)
	if !ok || !domain.ValidSpeedThroughput(ed.Throughput) {
		return nil, nil
	}
	return *ed.Throughput, nil
}
