package main

import "testing"

func TestAuditDetectsCurrentProcessLocalRegistry(t *testing.T) {
	report := runAudit()
	if report.Status != "FAIL" {
		t.Fatalf("status = %s, want FAIL until registry is canonical", report.Status)
	}
	if len(report.Checks) != 3 {
		t.Fatalf("checks = %d, want 3", len(report.Checks))
	}
	for _, item := range report.Checks {
		if item.Status != "FAIL" {
			t.Fatalf("check %s = %s, want FAIL for current implementation", item.Name, item.Status)
		}
	}
}
