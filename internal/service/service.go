// Package service implements staff authentication and hospital-scoped search.
package service

import (
	"context"
	"errors"
	"strconv"
	"time"

	"agnos/internal/domain"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const Issuer = "agnos"
const Audience = "agnos-api"

type Service struct {
	repo   domain.Repository
	his    domain.HISClient
	secret []byte
}
type Token struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func New(repo domain.Repository, his domain.HISClient, jwtSecret string) *Service {
	return &Service{repo, his, []byte(jwtSecret)}
}
func internal() error     { return domain.E("internal_error", "Internal server error") }
func unauthorized() error { return domain.E("unauthorized", "Invalid credentials or token") }
func (s *Service) CreateStaff(ctx context.Context, c domain.Credentials) (domain.Staff, error) {
	h, err := s.repo.FindHospital(ctx, c.Hospital)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Staff{}, domain.E("validation_error", "Unknown hospital")
	}
	if err != nil {
		return domain.Staff{}, internal()
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(c.Password), bcrypt.DefaultCost)
	if err != nil {
		return domain.Staff{}, internal()
	}
	staff, err := s.repo.CreateStaff(ctx, domain.Staff{HospitalID: h.ID, Username: c.Username, PasswordHash: string(hash)})
	if errors.Is(err, domain.ErrConflict) {
		return domain.Staff{}, domain.E("conflict", "Username already exists in this hospital")
	}
	if err != nil {
		return domain.Staff{}, internal()
	}
	return staff, nil
}
func (s *Service) Login(ctx context.Context, c domain.Credentials) (Token, error) {
	h, err := s.repo.FindHospital(ctx, c.Hospital)
	if errors.Is(err, domain.ErrNotFound) {
		return Token{}, unauthorized()
	}
	if err != nil {
		return Token{}, internal()
	}
	staff, err := s.repo.FindStaff(ctx, h.ID, c.Username)
	if errors.Is(err, domain.ErrNotFound) {
		return Token{}, unauthorized()
	}
	if err != nil {
		return Token{}, internal()
	}
	if bcrypt.CompareHashAndPassword([]byte(staff.PasswordHash), []byte(c.Password)) != nil {
		return Token{}, unauthorized()
	}
	now := time.Now()
	claims := jwt.RegisteredClaims{Issuer: Issuer, Subject: strconv.FormatInt(staff.ID, 10), Audience: jwt.ClaimStrings{Audience}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour))}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return Token{}, internal()
	}
	return Token{raw, "Bearer", 3600}, nil
}
func (s *Service) Authenticate(ctx context.Context, raw string) (domain.Staff, error) {
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) { return s.secret, nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(Issuer), jwt.WithAudience(Audience), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return domain.Staff{}, unauthorized()
	}
	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || id <= 0 {
		return domain.Staff{}, unauthorized()
	}
	staff, err := s.repo.StaffByID(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Staff{}, unauthorized()
	}
	if err != nil {
		return domain.Staff{}, internal()
	}
	if staff.HospitalID <= 0 {
		return domain.Staff{}, unauthorized()
	}
	return staff, nil
}
func (s *Service) Search(ctx context.Context, staff domain.Staff, f domain.SearchFilters) (domain.SearchResult, error) {
	f.PatientID = 0
	source := "local"
	if f.NationalID != nil || f.PassportID != nil {
		source = "his"
		field := "national_id"
		id := f.NationalID
		if id == nil {
			field = "passport_id"
			id = f.PassportID
		}
		h, err := s.repo.HospitalByID(ctx, staff.HospitalID)
		if err != nil {
			return domain.SearchResult{}, internal()
		}
		p, err := s.his.Lookup(ctx, h.Code, *id, field)
		if errors.Is(err, domain.ErrNotFound) {
			return domain.SearchResult{Data: []domain.Patient{}, Total: 0, Page: f.Page, PageSize: f.PageSize, Source: source}, nil
		}
		if err != nil {
			var safe *domain.Error
			if errors.As(err, &safe) {
				return domain.SearchResult{}, safe
			}
			return domain.SearchResult{}, domain.E("upstream_error", "HIS request failed")
		}
		p.HospitalID = staff.HospitalID
		p.ID = 0
		p, err = s.repo.UpsertPatient(ctx, p)
		if err != nil {
			return domain.SearchResult{}, internal()
		}
		if p.ID <= 0 {
			return domain.SearchResult{}, internal()
		}
		f.PatientID = p.ID
	}
	result, err := s.repo.SearchPatients(ctx, staff.HospitalID, f)
	if err != nil {
		return domain.SearchResult{}, internal()
	}
	if result.Data == nil {
		result.Data = []domain.Patient{}
	}
	result.Source = source
	result.Page = f.Page
	result.PageSize = f.PageSize
	return result, nil
}
