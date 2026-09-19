// Command mockhis provides synthetic HIS records for local demonstration only.
package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"
)

// patient is the external HIS contract: no database or hospital IDs.
type patient struct {
	FirstNameTH  *string `json:"first_name_th"`
	MiddleNameTH *string `json:"middle_name_th"`
	LastNameTH   *string `json:"last_name_th"`
	FirstNameEN  *string `json:"first_name_en"`
	MiddleNameEN *string `json:"middle_name_en"`
	LastNameEN   *string `json:"last_name_en"`
	DateOfBirth  *string `json:"date_of_birth"`
	PatientHN    string  `json:"patient_hn"`
	NationalID   *string `json:"national_id"`
	PassportID   *string `json:"passport_id"`
	PhoneNumber  *string `json:"phone_number"`
	Email        *string `json:"email"`
	Gender       *string `json:"gender"`
}

func ptr(value string) *string { return &value }

func fixtures() map[string][]patient {
	return map[string][]patient{
		"a": {
			{PatientHN: "A001", FirstNameTH: ptr("สมชาย"), MiddleNameTH: ptr("ทดสอบ"), LastNameTH: ptr("ใจดี"), FirstNameEN: ptr("Somchai"), MiddleNameEN: ptr("Demo"), LastNameEN: ptr("Jaidee"), DateOfBirth: ptr("1990-01-15"), NationalID: ptr("0000000000001"), PassportID: ptr("DEMO-SHARED"), PhoneNumber: ptr("0800000001"), Email: ptr("a001@example.test"), Gender: ptr("M")},
			{PatientHN: "A002", FirstNameTH: ptr("ทดสอบ"), FirstNameEN: ptr(`Literal%_\Name`), LastNameEN: ptr("Example"), NationalID: ptr("0000000000002")},
			{PatientHN: "A900", FirstNameTH: ptr("นำเข้า"), FirstNameEN: ptr("Imported"), LastNameEN: ptr("HospitalA"), NationalID: ptr("0000000000900"), PassportID: ptr("HIS-A900")},
		},
		"b": {
			{PatientHN: "B001", FirstNameTH: ptr("สมชาย"), LastNameTH: ptr("อื่น"), FirstNameEN: ptr("Somchai"), LastNameEN: ptr("Other"), DateOfBirth: ptr("1985-02-20"), NationalID: ptr("0000000000001"), PassportID: ptr("DEMO-SHARED"), PhoneNumber: ptr("0800000002"), Email: ptr("b001@example.test"), Gender: ptr("M")},
			{PatientHN: "B900", FirstNameTH: ptr("นำเข้า"), FirstNameEN: ptr("Imported"), LastNameEN: ptr("HospitalB"), NationalID: ptr("0000000000900"), PassportID: ptr("HIS-B900")},
		},
	}
}

func handler() http.Handler {
	data := fixtures()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /{hospital}/patient/search/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		identifier := r.PathValue("id")
		for _, p := range data[r.PathValue("hospital")] {
			if (p.NationalID != nil && *p.NationalID == identifier) || (p.PassportID != nil && *p.PassportID == identifier) {
				_ = json.NewEncoder(w).Encode(p)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not_found"}`))
	})
	return mux
}

func main() {
	server := &http.Server{Addr: ":8080", Handler: handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal("mock HIS server failed")
	}
}
