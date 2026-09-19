package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSyntheticHISContract(t *testing.T) {
	h := handler()
	for _, tc := range []struct{ path, hn string }{
		{"/a/patient/search/DEMO-SHARED", "A001"},
		{"/b/patient/search/DEMO-SHARED", "B001"},
		{"/a/patient/search/HIS-A900", "A900"},
		{"/b/patient/search/HIS-B900", "B900"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d", response.Code)
			}
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["patient_hn"] != tc.hn {
				t.Fatalf("patient_hn = %v", body["patient_hn"])
			}
			for _, private := range []string{"id", "hospital_id"} {
				if _, exists := body[private]; exists {
					t.Errorf("private field %s leaked", private)
				}
			}
			if len(body) != 13 {
				t.Errorf("HIS field count = %d, want 13", len(body))
			}
		})
	}
	for _, path := range []string{"/a/patient/search/HIS-B900", "/b/patient/search/HIS-A900", "/c/patient/search/DEMO-SHARED", "/a/patient/search/missing"} {
		response := httptest.NewRecorder()
		h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("%s status = %d", path, response.Code)
		}
	}
}
