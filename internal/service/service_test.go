package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"agnos/internal/domain"
	"github.com/golang-jwt/jwt/v5"
)

type repositoryStub struct {
	domain.Repository
	staff       domain.Staff
	err         error
	hospitalErr error
	upsertErr   error
	searchErr   error
}

func (r repositoryStub) StaffByID(context.Context, int64) (domain.Staff, error) {
	return r.staff, r.err
}
func (r repositoryStub) HospitalByID(context.Context, int64) (domain.Hospital, error) {
	return domain.Hospital{ID: 1, Code: "A"}, r.hospitalErr
}
func (r repositoryStub) UpsertPatient(context.Context, domain.Patient) (domain.Patient, error) {
	return domain.Patient{ID: 1}, r.upsertErr
}
func (r repositoryStub) SearchPatients(context.Context, int64, domain.SearchFilters) (domain.SearchResult, error) {
	return domain.SearchResult{}, r.searchErr
}

type hisStub struct{}

func (hisStub) Lookup(context.Context, string, string, string) (domain.Patient, error) {
	return domain.Patient{PatientHN: "HN"}, nil
}
func TestAuthenticateRequiresStrictClaims(t *testing.T) {
	r := repositoryStub{staff: domain.Staff{ID: 1, HospitalID: 2}}
	s := New(r, hisStub{}, "secret")
	for _, tc := range []struct {
		name   string
		mutate func(*jwt.RegisteredClaims)
		method jwt.SigningMethod
		key    string
		want   bool
	}{
		{"valid", func(*jwt.RegisteredClaims) {}, jwt.SigningMethodHS256, "secret", true},
		{"issuer", func(c *jwt.RegisteredClaims) { c.Issuer = "evil" }, jwt.SigningMethodHS256, "secret", false},
		{"audience", func(c *jwt.RegisteredClaims) { c.Audience = jwt.ClaimStrings{"evil"} }, jwt.SigningMethodHS256, "secret", false},
		{"missing expiry", func(c *jwt.RegisteredClaims) { c.ExpiresAt = nil }, jwt.SigningMethodHS256, "secret", false},
		{"future not before", func(c *jwt.RegisteredClaims) { c.NotBefore = jwt.NewNumericDate(time.Now().Add(time.Hour)) }, jwt.SigningMethodHS256, "secret", false},
		{"invalid subject", func(c *jwt.RegisteredClaims) { c.Subject = "-1" }, jwt.SigningMethodHS256, "secret", false},
		{"wrong algorithm", func(*jwt.RegisteredClaims) {}, jwt.SigningMethodHS384, "secret", false},
		{"wrong signature", func(*jwt.RegisteredClaims) {}, jwt.SigningMethodHS256, "wrong", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := jwt.RegisteredClaims{Issuer: Issuer, Audience: jwt.ClaimStrings{Audience}, Subject: "1", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}
			tc.mutate(&claims)
			raw, err := jwt.NewWithClaims(tc.method, claims).SignedString([]byte(tc.key))
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.Authenticate(context.Background(), raw)
			if (err == nil) != tc.want {
				t.Fatalf("unexpected authentication: %v", err)
			}
		})
	}
}
func TestSearchSanitizesStorageFailures(t *testing.T) {
	id := "123"
	secret := errors.New("database secret")
	for _, r := range []repositoryStub{{hospitalErr: secret}, {upsertErr: secret}, {searchErr: secret}} {
		s := New(r, hisStub{}, "secret")
		_, err := s.Search(context.Background(), domain.Staff{HospitalID: 1}, domain.SearchFilters{NationalID: &id, Page: 1, PageSize: 20})
		var safe *domain.Error
		if !errors.As(err, &safe) || safe.Code != "internal_error" || safe.Message != "Internal server error" {
			t.Fatalf("unsafe error %v", err)
		}
	}
}
