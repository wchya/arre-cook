package main

import (
	"bytes"
	"encoding/json"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validInput() []byte {
	energy := 100.0
	rows := []services.CatalogFoodInput{{Name: "测试数据", BasisUnit: "g", FoodState: "raw", EdibleBasis: "edible_portion", Nutrients: models.NutrientValues{EnergyKcal: &energy}, Provenance: models.CatalogProvenance{Dataset: "test", Version: "1", RecordID: "1", URL: "https://example.org/test", License: "synthetic test", ReviewedBy: "test", ReviewedAt: "2026-01-01"}}}
	data, _ := json.Marshal(rows)
	return data
}
func TestCatalogDefaultValidationNeverOpensDatabase(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "input.json")
	db := filepath.Join(dir, "must-not-exist.db")
	if err := os.WriteFile(file, validInput(), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"-file", file, "-sqlite", db}, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(db); !os.IsNotExist(err) {
		t.Fatal("validation opened database")
	}
	if !strings.Contains(out.String(), "no database opened") {
		t.Fatal("missing dry-run output")
	}
	if err := run([]string{"-file", file, "-sqlite", db, "-apply", "-migrate"}, &out); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-file", file, "-sqlite", db, "-apply"}, &out); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-sqlite", db, "-withdraw", "1", "-apply"}, &out); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-sqlite", db, "-withdraw", "1", "-apply"}, &out); err != nil {
		t.Fatalf("repeated withdrawal should succeed: %v", err)
	}
	out.Reset()
	if err := run([]string{"-sqlite", db, "-withdraw", "9999", "-apply"}, &out); err == nil || out.Len() != 0 {
		t.Fatal("unknown ID must fail without a success message")
	}
}

func TestCatalogRejectsMigrationWithoutApply(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"-migrate"}, &out); err == nil || !strings.Contains(err.Error(), "-apply") {
		t.Fatal("migration without explicit apply was not rejected")
	}
}
func TestCatalogRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	for _, data := range []string{string(validInput()) + " []", strings.Replace(string(validInput()), "\"name\":", "\"bogus\":0,\"name\":", 1), "[]", "null"} {
		if _, err := decodeCatalog(strings.NewReader(data)); err == nil {
			t.Fatal("invalid JSON accepted")
		}
	}
}
