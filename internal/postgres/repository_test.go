package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"agnos/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ptr(s string) *string { return &s }
func TestLiteralPattern(t *testing.T) {
	if got := literalPattern(` %_\ `); got != `%\%\_\\%` {
		t.Fatalf("got %q", got)
	}
}

// Each integration run gets an isolated schema; TEST_DATABASE_URL must permit CREATE SCHEMA.
func TestRepositoryIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run real PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("agnos_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	migration, err := os.ReadFile("../../migrations/001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	r := New(pool)
	a, err := r.FindHospital(ctx, "A")
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.FindHospital(ctx, "B")
	if err != nil {
		t.Fatal(err)
	}
	staff, err := r.CreateStaff(ctx, domain.Staff{HospitalID: a.ID, Username: "same", PasswordHash: "test-hash"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.CreateStaff(ctx, domain.Staff{HospitalID: a.ID, Username: "same", PasswordHash: "test-hash"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate staff: %v", err)
	}
	if _, err = r.CreateStaff(ctx, domain.Staff{HospitalID: b.ID, Username: "same", PasswordHash: "test-hash"}); err != nil {
		t.Fatal(err)
	}
	if got, e := r.FindStaff(ctx, a.ID, "same"); e != nil || got.ID != staff.ID {
		t.Fatalf("staff: %+v %v", got, e)
	}
	if _, e := r.FindStaff(ctx, b.ID, "missing"); !errors.Is(e, domain.ErrNotFound) {
		t.Fatal(e)
	}
	if got, e := r.StaffByID(ctx, staff.ID); e != nil || got.HospitalID != a.ID {
		t.Fatalf("principal: %+v %v", got, e)
	}
	if got, e := r.HospitalByID(ctx, b.ID); e != nil || got.Code != "B" {
		t.Fatalf("hospital: %+v %v", got, e)
	}
	p := domain.Patient{HospitalID: a.ID, PatientHN: "same-hn", FirstNameTH: ptr("สมชาย"), FirstNameEN: ptr(`Literal%_\Name`), MiddleNameEN: ptr("Middle"), LastNameEN: ptr("Surname"), DateOfBirth: ptr("2000-02-29"), NationalID: ptr("shared"), PassportID: ptr("shared-pass"), PhoneNumber: ptr("0800000001"), Email: ptr("test@example.test"), Gender: ptr("M")}
	original, err := r.UpsertPatient(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	p.HospitalID = b.ID
	other, err := r.UpsertPatient(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if original.ID == other.ID {
		t.Fatal("cross-hospital HN collided")
	}
	search := func(f domain.SearchFilters, want int64) {
		t.Helper()
		f.Page = 1
		f.PageSize = 20
		got, e := r.SearchPatients(ctx, a.ID, f)
		if e != nil {
			t.Fatal(e)
		}
		if got.Total != want {
			t.Fatalf("filter %+v: got total %d want %d", f, got.Total, want)
		}
		for _, row := range got.Data {
			if row.HospitalID != a.ID {
				t.Fatal("tenant leak")
			}
			if row.DateOfBirth != nil && *row.DateOfBirth != "2000-02-29" {
				t.Fatal("date conversion changed")
			}
		}
	}
	search(domain.SearchFilters{}, 1)
	for _, f := range []domain.SearchFilters{
		{NationalID: ptr(" shared ")}, {PassportID: ptr("shared-pass")}, {FirstName: ptr("สม")}, {FirstName: ptr(`%_\`)}, {FirstName: ptr("literal")}, {MiddleName: ptr("MID")}, {LastName: ptr("name")}, {DateOfBirth: ptr("2000-02-29")}, {PhoneNumber: ptr("0800000001")}, {Email: ptr("test@example.test")}, {PatientID: original.ID},
	} {
		search(f, 1)
	}
	for _, f := range []domain.SearchFilters{
		{NationalID: ptr("absent")}, {PassportID: ptr("absent")}, {FirstName: ptr("absent")}, {MiddleName: ptr("absent")}, {LastName: ptr("absent")}, {DateOfBirth: ptr("2000-02-28")}, {PhoneNumber: ptr("0800000002")}, {Email: ptr("absent")}, {PatientID: other.ID}, {FirstName: ptr("literal"), LastName: ptr("absent")}, {FirstName: ptr(`' OR true --`)},
	} {
		search(f, 0)
	}
	// Upsert refreshes the same row and clears nullable fields without touching B.
	updated, err := r.UpsertPatient(ctx, domain.Patient{HospitalID: a.ID, PatientHN: "same-hn", NationalID: ptr("new-id")})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != original.ID || updated.DateOfBirth != nil || updated.FirstNameEN != nil {
		t.Fatalf("bad update %+v", updated)
	}
	search(domain.SearchFilters{NationalID: ptr("shared")}, 0)
	got, err := r.SearchPatients(ctx, b.ID, domain.SearchFilters{Page: 1, PageSize: 20, NationalID: ptr("shared")})
	if err != nil || got.Total != 1 {
		t.Fatalf("B modified: %+v %v", got, err)
	}
	for i := 0; i < 3; i++ {
		if _, e := r.UpsertPatient(ctx, domain.Patient{HospitalID: a.ID, PatientHN: fmt.Sprintf("page-%d", i)}); e != nil {
			t.Fatal(e)
		}
	}
	page1, e := r.SearchPatients(ctx, a.ID, domain.SearchFilters{Page: 1, PageSize: 2})
	if e != nil {
		t.Fatal(e)
	}
	page2, e := r.SearchPatients(ctx, a.ID, domain.SearchFilters{Page: 2, PageSize: 2})
	if e != nil {
		t.Fatal(e)
	}
	page3, e := r.SearchPatients(ctx, a.ID, domain.SearchFilters{Page: 3, PageSize: 2})
	if e != nil {
		t.Fatal(e)
	}
	if page1.Total != 4 || page2.Total != 4 || page3.Total != 4 || len(page1.Data) != 2 || len(page2.Data) != 2 || len(page3.Data) != 0 || page1.Data[1].ID >= page2.Data[0].ID {
		t.Fatal("unstable or incorrect pagination")
	}
	for _, fixture := range []domain.Patient{
		{HospitalID: a.ID, PatientHN: "literal", FirstNameEN: ptr(`Only%_\marker`)},
		{HospitalID: a.ID, PatientHN: "ordinary", FirstNameEN: ptr("Ordinary")},
	} {
		if _, e := r.UpsertPatient(ctx, fixture); e != nil {
			t.Fatal(e)
		}
	}
	for _, literal := range []string{"%", "_", `\`} {
		search(domain.SearchFilters{FirstName: ptr(literal)}, 1)
	}
	if _, e = r.UpsertPatient(ctx, domain.Patient{HospitalID: a.ID, PatientHN: "bad", Gender: ptr("X")}); e == nil {
		t.Fatal("gender constraint missing")
	}
	if _, e = r.UpsertPatient(ctx, domain.Patient{HospitalID: a.ID, PatientHN: strings.Repeat(" ", 3)}); e == nil {
		t.Fatal("HN constraint missing")
	}
	if _, e = r.UpsertPatient(ctx, domain.Patient{HospitalID: 999999, PatientHN: "orphan"}); e == nil {
		t.Fatal("hospital FK missing")
	}
}
