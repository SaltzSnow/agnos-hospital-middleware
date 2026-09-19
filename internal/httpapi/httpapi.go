// Package httpapi exposes the JSON API with strict validation and safe errors.
package httpapi

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"

	"agnos/internal/domain"
	"agnos/internal/service"
	"github.com/gin-gonic/gin"
)

var usernameRE = regexp.MustCompile(`^[A-Za-z0-9._-]{3,64}$`)
var identifierRE = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

func New(s *service.Service, registrationKey string) *gin.Engine {
	r := gin.New()
	r.Use(safeRecovery())
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.POST("/staff/create", func(c *gin.Context) {
		if registrationKey == "" || subtle.ConstantTimeCompare([]byte(c.GetHeader("X-Registration-Key")), []byte(registrationKey)) != 1 {
			fail(c, domain.E("forbidden", "Invalid registration key"))
			return
		}
		creds, err := credentials(c)
		if err != nil {
			fail(c, err)
			return
		}
		staff, err := s.CreateStaff(c.Request.Context(), creds)
		if err != nil {
			fail(c, err)
			return
		}
		c.JSON(201, staff)
	})
	r.POST("/staff/login", func(c *gin.Context) {
		creds, err := credentials(c)
		if err != nil {
			fail(c, err)
			return
		}
		token, err := s.Login(c.Request.Context(), creds)
		if err != nil {
			fail(c, err)
			return
		}
		c.JSON(200, token)
	})
	r.POST("/patient/search", func(c *gin.Context) {
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			fail(c, domain.E("unauthorized", "Bearer token required"))
			return
		}
		staff, err := s.Authenticate(c.Request.Context(), parts[1])
		if err != nil {
			fail(c, err)
			return
		}
		f, err := filters(c)
		if err != nil {
			fail(c, err)
			return
		}
		result, err := s.Search(c.Request.Context(), staff, f)
		if err != nil {
			fail(c, err)
			return
		}
		c.JSON(200, result)
	})
	return r
}

// safeRecovery deliberately avoids Gin's request dump, which can expose the
// registration credential and patient data when a handler panics.
func safeRecovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recover() != nil {
				c.Abort()
				if !c.Writer.Written() {
					fail(c, domain.E("internal_error", "Internal server error"))
				}
			}
		}()
		c.Next()
	}
}
func invalid(message string) error { return domain.E("validation_error", message) }
func decode(c *gin.Context, dst any) (map[string]json.RawMessage, error) {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		return nil, invalid("Content-Type must be application/json")
	}
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10))
	if err != nil {
		return nil, invalid("Request body exceeds limit or cannot be read")
	}
	// Read keys explicitly because encoding/json otherwise silently accepts duplicates.
	d := json.NewDecoder(bytes.NewReader(raw))
	tok, err := d.Token()
	if err != nil || tok != json.Delim('{') {
		return nil, invalid("Body must be a JSON object")
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return nil, invalid("Invalid JSON")
		}
		k, ok := key.(string)
		if !ok {
			return nil, invalid("Invalid JSON")
		}
		if _, exists := fields[k]; exists {
			return nil, invalid("Duplicate JSON field")
		}
		var v json.RawMessage
		if d.Decode(&v) != nil {
			return nil, invalid("Invalid JSON")
		}
		fields[k] = v
	}
	if _, err = d.Token(); err != nil {
		return nil, invalid("Invalid JSON")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, invalid("Trailing JSON is not allowed")
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(dst); err != nil {
		return nil, invalid("Unknown field or invalid field type")
	}
	return fields, nil
}
func credentials(c *gin.Context) (domain.Credentials, error) {
	var out domain.Credentials
	fields, err := decode(c, &out)
	if err != nil {
		return out, err
	}
	for k := range fields {
		if k != "username" && k != "password" && k != "hospital" {
			return out, invalid("Unknown field")
		}
	}
	for _, k := range []string{"username", "password", "hospital"} {
		v, ok := fields[k]
		if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return out, invalid("Username, password and hospital are required")
		}
	}
	out.Username = strings.TrimSpace(out.Username)
	out.Hospital = strings.TrimSpace(out.Hospital)
	if !usernameRE.MatchString(out.Username) {
		return out, invalid("Username must be 3-64 ASCII letters, digits, dots, underscores or dashes")
	}
	if len(out.Password) < 8 || len(out.Password) > 72 {
		return out, invalid("Password must be 8-72 bytes")
	}
	if !identifierRE.MatchString(out.Hospital) {
		return out, invalid("Invalid hospital")
	}
	return out, nil
}
func filters(c *gin.Context) (domain.SearchFilters, error) {
	f := domain.SearchFilters{Page: 1, PageSize: 20}
	fields, err := decode(c, &f)
	if err != nil {
		return f, err
	}
	// Check exact names, also disallowing encoding/json's case-insensitive aliases.
	allowed := map[string]bool{"national_id": true, "passport_id": true, "first_name": true, "middle_name": true, "last_name": true, "date_of_birth": true, "phone_number": true, "email": true, "page": true, "page_size": true}
	for k, v := range fields {
		if !allowed[k] {
			return f, invalid("Unknown field")
		}
		if bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return f, invalid("Null fields are not allowed")
		}
	}
	for _, p := range []*string{f.NationalID, f.PassportID, f.FirstName, f.MiddleName, f.LastName, f.DateOfBirth, f.PhoneNumber, f.Email} {
		if p != nil {
			*p = strings.TrimSpace(*p)
			if *p == "" || len(*p) > 255 {
				return f, invalid("Filters must contain 1-255 bytes")
			}
		}
	}
	for _, p := range []*string{f.NationalID, f.PassportID} {
		if p != nil && !identifierRE.MatchString(*p) {
			return f, invalid("Invalid identifier")
		}
	}
	if f.DateOfBirth != nil {
		d, err := time.Parse("2006-01-02", *f.DateOfBirth)
		if err != nil || d.Year() < 1 || d.Format("2006-01-02") != *f.DateOfBirth {
			return f, invalid("Date must be a real YYYY-MM-DD date")
		}
	}
	if f.Page < 1 || f.Page > 1000000 || f.PageSize < 1 || f.PageSize > 100 {
		return f, invalid("Invalid pagination")
	}
	return f, nil
}
func fail(c *gin.Context, err error) {
	e := &domain.Error{Code: "internal_error", Message: "Internal server error"}
	var safe *domain.Error
	if errors.As(err, &safe) {
		e = safe
	}
	statuses := map[string]int{"validation_error": 400, "unauthorized": 401, "forbidden": 403, "conflict": 409, "upstream_error": 502, "upstream_timeout": 504, "upstream_unavailable": 503, "his_unavailable": 503}
	status, ok := statuses[e.Code]
	if !ok {
		status = 500
	}
	body := gin.H{"code": e.Code, "message": e.Message}
	if e.Field != "" {
		body["field"] = e.Field
	}
	c.JSON(status, gin.H{"error": body})
}
