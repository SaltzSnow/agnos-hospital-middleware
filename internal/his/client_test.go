package his

import (
	"agnos/internal/domain"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLookupResponses(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       string
	}{
		{"minimal", `{"patient_hn":"A1","national_id":"123"}`, 200, ""},
		{"null demographics", `{"patient_hn":"A1","national_id":"123","gender":null,"date_of_birth":null,"email":null}`, 200, ""},
		{"extra fields", `{"patient_hn":"A1","national_id":"123","harmless":true,"id":99,"hospital_id":99}`, 200, ""},
		{"valid date", `{"patient_hn":"A1","national_id":"123","date_of_birth":"2000-02-29","gender":"F"}`, 200, ""},
		{"year zero", `{"patient_hn":"A1","national_id":"123","date_of_birth":"0000-01-01"}`, 200, "upstream_error"},
		{"invalid date", `{"patient_hn":"A1","national_id":"123","date_of_birth":"2001-02-29"}`, 200, "upstream_error"},
		{"gender", `{"patient_hn":"A1","national_id":"123","gender":"X"}`, 200, "upstream_error"},
		{"numeric id", `{"patient_hn":"A1","national_id":123}`, 200, "upstream_error"},
		{"wrong type", `{"patient_hn":"A1","national_id":"123","email":true}`, 200, "upstream_error"},
		{"wrong identifier", `{"patient_hn":"A1","national_id":"999"}`, 200, "upstream_error"},
		{"missing identifier", `{"patient_hn":"A1"}`, 200, "upstream_error"},
		{"blank HN", `{"patient_hn":" ","national_id":"123"}`, 200, "upstream_error"},
		{"null HN", `{"patient_hn":null,"national_id":"123"}`, 200, "upstream_error"},
		{"array", `[]`, 200, "upstream_error"}, {"null", `null`, 200, "upstream_error"}, {"malformed", `{`, 200, "upstream_error"},
		{"trailing JSON", `{"patient_hn":"A1","national_id":"123"}{}`, 200, "upstream_error"},
		{"oversized padded JSON hides trailing invalid bytes", `{"patient_hn":"A1","national_id":"123"}` + strings.Repeat(" ", maxResponseBytes) + `invalid`, 200, "upstream_error"},
		{"oversized", `{"padding":"` + strings.Repeat("a", maxResponseBytes) + `"}`, 200, "upstream_error"},
		{"not found", "", 404, "notfound"}, {"failure", "secret response", 500, "upstream_error"}, {"redirect", "", 302, "upstream_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			p, err := New(map[string]string{"A": server.URL}, time.Second).Lookup(context.Background(), "A", "123", "national_id")
			if tc.want == "" {
				if err != nil || p.PatientHN != "A1" || p.ID != 0 || p.HospitalID != 0 {
					t.Fatalf("patient=%+v err=%v", p, err)
				}
				return
			}
			if tc.want == "notfound" {
				if !errors.Is(err, domain.ErrNotFound) {
					t.Fatal(err)
				}
				return
			}
			var e *domain.Error
			if !errors.As(err, &e) || e.Code != tc.want {
				t.Fatalf("got %v want %s", err, tc.want)
			}
		})
	}
}
func TestTimeoutAndConfiguration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	c := New(map[string]string{"A": server.URL}, 20*time.Millisecond)
	for _, tc := range []struct{ hospital, code string }{{"A", "upstream_timeout"}, {"B", "his_unavailable"}} {
		_, err := c.Lookup(context.Background(), tc.hospital, "123", "national_id")
		var e *domain.Error
		if !errors.As(err, &e) || e.Code != tc.code {
			t.Fatalf("got %v want %s", err, tc.code)
		}
	}
}
func TestEscapedPathAndPassport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/patient/search/A%2FB%3FC" || r.URL.RawQuery != "" {
			t.Errorf("unexpected URL %s", r.URL)
		}
		fmt.Fprint(w, `{"patient_hn":"A1","passport_id":"A/B?C"}`)
	}))
	defer server.Close()
	if _, err := New(map[string]string{"A": server.URL}, time.Second).Lookup(context.Background(), "A", "A/B?C", "passport_id"); err != nil {
		t.Fatal(err)
	}
}
func TestRedirectNotFollowed(t *testing.T) {
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("redirect followed") }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 302) }))
	defer server.Close()
	_, err := New(map[string]string{"A": server.URL}, time.Second).Lookup(context.Background(), "A", "123", "national_id")
	if err == nil {
		t.Fatal("expected redirect error")
	}
}
