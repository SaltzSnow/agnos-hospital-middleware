package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agnos/internal/domain"
	"agnos/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type fakeRepo struct {
	staff                domain.Staff
	created              bool
	searchHospital       int64
	filter               domain.SearchFilters
	searchCalls, upserts int
	failure              error
}

func (r *fakeRepo) FindHospital(context.Context, string) (domain.Hospital, error) {
	return domain.Hospital{ID: 1, Code: "A"}, r.failure
}
func (r *fakeRepo) HospitalByID(context.Context, int64) (domain.Hospital, error) {
	return domain.Hospital{ID: 1, Code: "A"}, r.failure
}
func (r *fakeRepo) CreateStaff(_ context.Context, s domain.Staff) (domain.Staff, error) {
	if r.created {
		return domain.Staff{}, domain.ErrConflict
	}
	s.ID = 7
	r.staff = s
	r.created = true
	return s, r.failure
}
func (r *fakeRepo) FindStaff(_ context.Context, _ int64, u string) (domain.Staff, error) {
	if u != r.staff.Username {
		return domain.Staff{}, domain.ErrNotFound
	}
	return r.staff, r.failure
}
func (r *fakeRepo) StaffByID(_ context.Context, id int64) (domain.Staff, error) {
	if id != r.staff.ID {
		return domain.Staff{}, domain.ErrNotFound
	}
	return r.staff, r.failure
}
func (r *fakeRepo) UpsertPatient(_ context.Context, p domain.Patient) (domain.Patient, error) {
	r.upserts++
	if p.HospitalID != r.staff.HospitalID {
		panic("wrong tenant")
	}
	p.ID = 99
	return p, r.failure
}
func (r *fakeRepo) SearchPatients(_ context.Context, h int64, f domain.SearchFilters) (domain.SearchResult, error) {
	r.searchCalls++
	r.searchHospital = h
	r.filter = f
	return domain.SearchResult{Data: []domain.Patient{}, Total: 0}, r.failure
}

type fakeHIS struct {
	err       error
	calls     int
	field, id string
}

func (h *fakeHIS) Lookup(_ context.Context, _ string, id, field string) (domain.Patient, error) {
	h.calls++
	h.id = id
	h.field = field
	return domain.Patient{HospitalID: 999, PatientHN: "HN1", NationalID: &id}, h.err
}
func setup(t *testing.T) (*gin.Engine, *fakeRepo, *fakeHIS, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := &fakeRepo{}
	h := &fakeHIS{}
	engine := New(service.New(r, h, "test-secret"), "registration-secret")
	w := request(engine, "/staff/create", `{"username":"staff","password":"password123","hospital":"A"}`, "", "registration-secret")
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "password") || strings.Contains(w.Body.String(), "hospital") {
		t.Fatal("staff secret leaked")
	}
	w = request(engine, "/staff/login", `{"username":"staff","password":"password123","hospital":"A"}`, "", "")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var token service.Token
	if json.Unmarshal(w.Body.Bytes(), &token) != nil || token.AccessToken == "" {
		t.Fatal("missing token")
	}
	return engine, r, h, token.AccessToken
}
func request(e *gin.Engine, path, body, token, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if key != "" {
		req.Header.Set("X-Registration-Key", key)
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	return w
}
func TestStaffEndpoints(t *testing.T) {
	e, r, _, _ := setup(t)
	for _, tc := range []struct {
		name, path, body, key string
		want                  int
	}{
		{"duplicate", "/staff/create", `{"username":"staff","password":"password123","hospital":"A"}`, "registration-secret", 409},
		{"missing key", "/staff/create", `{}`, "", 403}, {"wrong key", "/staff/create", `{}`, "bad", 403},
		{"short password", "/staff/create", `{"username":"other","password":"short","hospital":"A"}`, "registration-secret", 400},
		{"missing hospital", "/staff/create", `{"username":"other","password":"password123"}`, "registration-secret", 400},
		{"null username", "/staff/create", `{"username":null,"password":"password123","hospital":"A"}`, "registration-secret", 400},
		{"unknown", "/staff/create", `{"username":"other","password":"password123","hospital":"A","hospital_id":1}`, "registration-secret", 400},
		{"wrong password", "/staff/login", `{"username":"staff","password":"wrongpass","hospital":"A"}`, "", 401},
		{"missing staff", "/staff/login", `{"username":"nobody","password":"password123","hospital":"A"}`, "", 401},
		{"bad type", "/staff/login", `{"username":1,"password":"password123","hospital":"A"}`, "", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := request(e, tc.path, tc.body, "", tc.key)
			if w.Code != tc.want {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
		})
	}
	r.failure = errors.New("secret database error")
	w := request(e, "/staff/login", `{"username":"staff","password":"password123","hospital":"A"}`, "", "")
	if w.Code != 500 || strings.Contains(w.Body.String(), "database") {
		t.Fatal(w.Body.String())
	}
}
func TestSearchValidation(t *testing.T) {
	e, _, _, token := setup(t)
	for _, body := range []string{`{"hospital_id":2}`, `{"hospital":"B"}`, `{"PatientID":5}`, `{"id":5}`, `{"national_id":null}`, `{"email":""}`, `{"first_name":"  "}`, `{"first_name":5}`, `{"page":null}`, `{"page":0}`, `{"page":1000001}`, `{"page_size":101}`, `{"page_size":0}`, `{"page":1.5}`, `{"date_of_birth":"2023-02-29"}`, `{"date_of_birth":"0000-01-01"}`, `{"passport_id":"../path"}`, `{"page":1,"page":2}`, `{} {}`, `[]`, `null`, `{"FIRST_NAME":"x"}`, `{"first_name":"` + strings.Repeat("a", 256) + `"}`} {
		t.Run(body, func(t *testing.T) {
			w := request(e, "/patient/search", body, token, "")
			if w.Code != 400 {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
		})
	}
	for _, body := range []string{`{}`, `{"national_id":"123"}`, `{"passport_id":"P1"}`, `{"first_name":"สมชาย"}`, `{"middle_name":"Lee"}`, `{"last_name":"Doe"}`, `{"date_of_birth":"2024-02-29"}`, `{"phone_number":"123"}`, `{"email":"a@example.test"}`, `{"page":2,"page_size":100}`} {
		if w := request(e, "/patient/search", body, token, ""); w.Code != 200 {
			t.Fatalf("%s: %s", body, w.Body.String())
		}
	}
	req := httptest.NewRequest("POST", "/patient/search", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatal("accepted non JSON")
	}
	if w := request(e, "/patient/search", `{"first_name":"`+strings.Repeat("a", 65536)+`"}`, token, ""); w.Code != 400 {
		t.Fatal("accepted oversized body")
	}
}
func TestAuthAndTenant(t *testing.T) {
	e, r, _, token := setup(t)
	claims := jwt.RegisteredClaims{Issuer: service.Issuer, Audience: jwt.ClaimStrings{service.Audience}, Subject: "7", ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour))}
	expired, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-secret"))
	for _, raw := range []string{"", "wrong", expired, token[:len(token)-8] + "AAAAAAAA"} {
		if w := request(e, "/patient/search", `{}`, raw, ""); w.Code != 401 {
			t.Fatalf("got %d", w.Code)
		}
	}
	r.staff.HospitalID = 2
	if w := request(e, "/patient/search", `{}`, token, ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if r.searchHospital != 2 {
		t.Fatal("tenant was not re-read from staff")
	}
	r.staff.ID = 8
	if w := request(e, "/patient/search", `{}`, token, ""); w.Code != 401 {
		t.Fatal("deleted staff accepted")
	}
}
func TestSearchHISFlow(t *testing.T) {
	e, r, h, token := setup(t)
	if w := request(e, "/patient/search", `{"national_id":"123","passport_id":"P1","first_name":"Other"}`, token, ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if h.field != "national_id" || h.id != "123" || r.upserts != 1 || r.filter.PatientID != 99 || *r.filter.PassportID != "P1" || *r.filter.FirstName != "Other" {
		t.Fatal("HIS must precede AND query constrained to fetched row")
	}
	baseline := r.searchCalls
	for _, tc := range []struct {
		err  error
		want int
	}{{domain.ErrNotFound, 200}, {domain.E("upstream_error", "HIS request failed"), 502}, {domain.E("upstream_timeout", "HIS timed out"), 504}, {domain.E("upstream_unavailable", "HIS not configured"), 503}, {errors.New("raw upstream secret"), 502}} {
		h.err = tc.err
		w := request(e, "/patient/search", `{"national_id":"123"}`, token, "")
		if w.Code != tc.want {
			t.Fatalf("got %d: %s", w.Code, w.Body.String())
		}
		if r.searchCalls != baseline || r.upserts != 1 {
			t.Fatal("upstream failure fell back to local or persisted")
		}
		if strings.Contains(w.Body.String(), "secret") {
			t.Fatal("leaked upstream error")
		}
	}
}

func TestRecoveryDoesNotExposeRequestOrPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logs strings.Builder
	original := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logs
	t.Cleanup(func() { gin.DefaultErrorWriter = original })
	e := New(nil, "registration-secret")
	e.POST("/panic-test", func(c *gin.Context) {
		panic("sensitive patient and database detail")
	})
	w := request(e, "/panic-test", `{"patient":"sensitive"}`, "bearer-secret", "registration-secret")
	if w.Code != 500 || w.Body.String() != `{"error":{"code":"internal_error","message":"Internal server error"}}` {
		t.Fatalf("unexpected recovery response: %d %s", w.Code, w.Body.String())
	}
	if logs.Len() != 0 {
		t.Fatalf("recovery must not dump request or panic details: %s", logs.String())
	}
}
