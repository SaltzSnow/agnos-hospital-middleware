// Package his validates hospital responses before they can reach persistence.
package his

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"agnos/internal/domain"
)

const maxResponseBytes = 1 << 20

type Client struct {
	baseURLs map[string]string
	http     *http.Client
	timeout  time.Duration
}

func New(baseURLs map[string]string, timeout time.Duration) *Client {
	urls := make(map[string]string, len(baseURLs))
	for k, v := range baseURLs {
		urls[k] = v
	}
	return &Client{baseURLs: urls, timeout: timeout, http: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

var _ domain.HISClient = (*Client)(nil)

func upstreamError() error {
	return domain.E("upstream_error", "Hospital system returned an invalid response")
}
func (c *Client) Lookup(ctx context.Context, hospitalCode, id, identifierField string) (domain.Patient, error) {
	var p domain.Patient
	base := strings.TrimRight(c.baseURLs[hospitalCode], "/")
	if base == "" {
		return p, domain.E("his_unavailable", "Hospital system is not configured")
	}
	if identifierField != "national_id" && identifierField != "passport_id" {
		return p, upstreamError()
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/patient/search/"+url.PathEscape(id), nil)
	if err != nil {
		return p, upstreamError()
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return p, domain.E("upstream_timeout", "Hospital system timed out")
		}
		return p, upstreamError()
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return p, domain.ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return p, upstreamError()
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return p, domain.E("upstream_timeout", "Hospital system timed out")
		}
		return p, upstreamError()
	}
	if len(body) > maxResponseBytes {
		return p, upstreamError()
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 || body[0] != '{' {
		return p, upstreamError()
	}
	// A dedicated wire model prevents an upstream response from selecting local IDs.
	var wire struct {
		PatientHN    string  `json:"patient_hn"`
		FirstNameTH  *string `json:"first_name_th"`
		MiddleNameTH *string `json:"middle_name_th"`
		LastNameTH   *string `json:"last_name_th"`
		FirstNameEN  *string `json:"first_name_en"`
		MiddleNameEN *string `json:"middle_name_en"`
		LastNameEN   *string `json:"last_name_en"`
		DateOfBirth  *string `json:"date_of_birth"`
		NationalID   *string `json:"national_id"`
		PassportID   *string `json:"passport_id"`
		PhoneNumber  *string `json:"phone_number"`
		Email        *string `json:"email"`
		Gender       *string `json:"gender"`
	}
	if json.Unmarshal(body, &wire) != nil || strings.TrimSpace(wire.PatientHN) == "" {
		return p, upstreamError()
	}
	if wire.DateOfBirth != nil {
		d, e := time.Parse("2006-01-02", *wire.DateOfBirth)
		if e != nil || d.Year() < 1 || d.Format("2006-01-02") != *wire.DateOfBirth {
			return p, upstreamError()
		}
	}
	if wire.Gender != nil && *wire.Gender != "M" && *wire.Gender != "F" {
		return p, upstreamError()
	}
	identifier := wire.NationalID
	if identifierField == "passport_id" {
		identifier = wire.PassportID
	}
	if identifier == nil || *identifier != id {
		return p, upstreamError()
	}
	p = domain.Patient{PatientHN: wire.PatientHN, FirstNameTH: wire.FirstNameTH, MiddleNameTH: wire.MiddleNameTH, LastNameTH: wire.LastNameTH, FirstNameEN: wire.FirstNameEN, MiddleNameEN: wire.MiddleNameEN, LastNameEN: wire.LastNameEN, DateOfBirth: wire.DateOfBirth, NationalID: wire.NationalID, PassportID: wire.PassportID, PhoneNumber: wire.PhoneNumber, Email: wire.Email, Gender: wire.Gender}
	return p, nil
}
