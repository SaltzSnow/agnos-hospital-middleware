// Package domain defines the shared application contracts without transport dependencies.
package domain

import (
	"context"
	"errors"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

// Error exposes only safe, deliberate messages to API clients.
type Error struct {
	Code    string
	Message string
	Field   string
}

func (e *Error) Error() string      { return e.Message }
func E(code, message string) *Error { return &Error{Code: code, Message: message} }

type Hospital struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}
type Staff struct {
	ID           int64  `json:"id"`
	HospitalID   int64  `json:"-"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
}
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Hospital string `json:"hospital"`
}
type Patient struct {
	ID           int64   `json:"id"`
	HospitalID   int64   `json:"-"`
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

// SearchFilters is validated and normalized by the transport before entering the service.
// PatientID is an internal constraint applied after an authoritative HIS lookup.
type SearchFilters struct {
	NationalID  *string `json:"national_id"`
	PassportID  *string `json:"passport_id"`
	FirstName   *string `json:"first_name"`
	MiddleName  *string `json:"middle_name"`
	LastName    *string `json:"last_name"`
	DateOfBirth *string `json:"date_of_birth"`
	PhoneNumber *string `json:"phone_number"`
	Email       *string `json:"email"`
	Page        int     `json:"page"`
	PageSize    int     `json:"page_size"`
	PatientID   int64   `json:"-"`
}
type SearchResult struct {
	Data     []Patient `json:"data"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
	Source   string    `json:"source"`
}
type Repository interface {
	FindHospital(context.Context, string) (Hospital, error)
	HospitalByID(context.Context, int64) (Hospital, error)
	CreateStaff(context.Context, Staff) (Staff, error)
	FindStaff(context.Context, int64, string) (Staff, error)
	StaffByID(context.Context, int64) (Staff, error)
	UpsertPatient(context.Context, Patient) (Patient, error)
	SearchPatients(context.Context, int64, SearchFilters) (SearchResult, error)
}
type HISClient interface {
	Lookup(ctx context.Context, hospitalCode, identifier, identifierField string) (Patient, error)
}
