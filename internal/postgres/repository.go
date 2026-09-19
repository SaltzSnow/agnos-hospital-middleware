package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"agnos/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

var _ domain.Repository = (*Repository)(nil)

func translate(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.ErrConflict
	}
	return err
}
func (r *Repository) FindHospital(ctx context.Context, code string) (h domain.Hospital, err error) {
	err = r.pool.QueryRow(ctx, `SELECT id, code, name FROM hospitals WHERE code=$1`, code).Scan(&h.ID, &h.Code, &h.Name)
	return h, translate(err)
}
func (r *Repository) HospitalByID(ctx context.Context, id int64) (h domain.Hospital, err error) {
	err = r.pool.QueryRow(ctx, `SELECT id, code, name FROM hospitals WHERE id=$1`, id).Scan(&h.ID, &h.Code, &h.Name)
	return h, translate(err)
}
func (r *Repository) CreateStaff(ctx context.Context, s domain.Staff) (domain.Staff, error) {
	err := r.pool.QueryRow(ctx, `INSERT INTO staff(hospital_id,username,password_hash) VALUES($1,$2,$3) RETURNING id`, s.HospitalID, s.Username, s.PasswordHash).Scan(&s.ID)
	return s, translate(err)
}
func (r *Repository) FindStaff(ctx context.Context, hospitalID int64, username string) (s domain.Staff, err error) {
	err = r.pool.QueryRow(ctx, `SELECT id,hospital_id,username,password_hash FROM staff WHERE hospital_id=$1 AND username=$2`, hospitalID, username).Scan(&s.ID, &s.HospitalID, &s.Username, &s.PasswordHash)
	return s, translate(err)
}

// StaffByID resolves the authenticated principal before its hospital is known.
func (r *Repository) StaffByID(ctx context.Context, id int64) (s domain.Staff, err error) {
	err = r.pool.QueryRow(ctx, `SELECT id,hospital_id,username,password_hash FROM staff WHERE id=$1`, id).Scan(&s.ID, &s.HospitalID, &s.Username, &s.PasswordHash)
	return s, translate(err)
}

const patientColumns = `id,hospital_id,patient_hn,first_name_th,middle_name_th,last_name_th,first_name_en,middle_name_en,last_name_en,to_char(date_of_birth,'YYYY-MM-DD'),national_id,passport_id,phone_number,email,gender`

func scanPatient(row interface{ Scan(...any) error }) (p domain.Patient, err error) {
	err = row.Scan(&p.ID, &p.HospitalID, &p.PatientHN, &p.FirstNameTH, &p.MiddleNameTH, &p.LastNameTH, &p.FirstNameEN, &p.MiddleNameEN, &p.LastNameEN, &p.DateOfBirth, &p.NationalID, &p.PassportID, &p.PhoneNumber, &p.Email, &p.Gender)
	return p, translate(err)
}
func (r *Repository) UpsertPatient(ctx context.Context, p domain.Patient) (domain.Patient, error) {
	return scanPatient(r.pool.QueryRow(ctx, `INSERT INTO patients(hospital_id,patient_hn,first_name_th,middle_name_th,last_name_th,first_name_en,middle_name_en,last_name_en,date_of_birth,national_id,passport_id,phone_number,email,gender)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::text::date,$10,$11,$12,$13,$14)
 ON CONFLICT(hospital_id,patient_hn) DO UPDATE SET
 first_name_th=EXCLUDED.first_name_th,middle_name_th=EXCLUDED.middle_name_th,last_name_th=EXCLUDED.last_name_th,
 first_name_en=EXCLUDED.first_name_en,middle_name_en=EXCLUDED.middle_name_en,last_name_en=EXCLUDED.last_name_en,
 date_of_birth=EXCLUDED.date_of_birth,national_id=EXCLUDED.national_id,passport_id=EXCLUDED.passport_id,
 phone_number=EXCLUDED.phone_number,email=EXCLUDED.email,gender=EXCLUDED.gender RETURNING `+patientColumns,
		p.HospitalID, p.PatientHN, p.FirstNameTH, p.MiddleNameTH, p.LastNameTH, p.FirstNameEN, p.MiddleNameEN, p.LastNameEN, p.DateOfBirth, p.NationalID, p.PassportID, p.PhoneNumber, p.Email, p.Gender))
}
func literalPattern(s string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(s)) + "%"
}
func (r *Repository) SearchPatients(ctx context.Context, hospitalID int64, f domain.SearchFilters) (domain.SearchResult, error) {
	result := domain.SearchResult{Data: []domain.Patient{}, Page: f.Page, PageSize: f.PageSize}
	if f.Page < 1 || f.PageSize < 1 || f.PageSize > 100 || f.Page > 1000000 {
		return result, domain.E("validation_error", "Invalid pagination")
	}
	args := []any{hospitalID}
	where := []string{"hospital_id=$1"}
	add := func(expr string, v any) { args = append(args, v); where = append(where, fmt.Sprintf(expr, len(args))) }
	if f.PatientID != 0 {
		add("id=$%d", f.PatientID)
	}
	for _, v := range []struct {
		column string
		value  *string
	}{{"national_id", f.NationalID}, {"passport_id", f.PassportID}, {"date_of_birth", f.DateOfBirth}, {"phone_number", f.PhoneNumber}, {"email", f.Email}} {
		if v.value != nil {
			cast := ""
			if v.column == "date_of_birth" {
				cast = "::text::date"
			}
			add(v.column+"=$%d"+cast, strings.TrimSpace(*v.value))
		}
	}
	for _, v := range []struct {
		column string
		value  *string
	}{{"first_name", f.FirstName}, {"middle_name", f.MiddleName}, {"last_name", f.LastName}} {
		if v.value != nil {
			args = append(args, literalPattern(*v.value))
			n := len(args)
			where = append(where, fmt.Sprintf("(%s_th ILIKE $%d ESCAPE E'\\\\' OR %s_en ILIKE $%d ESCAPE E'\\\\')", v.column, n, v.column, n))
		}
	}
	clause := strings.Join(where, " AND ")
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM patients WHERE "+clause, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	args = append(args, f.PageSize, int64(f.Page-1)*int64(f.PageSize))
	rows, err := tx.Query(ctx, "SELECT "+patientColumns+" FROM patients WHERE "+clause+fmt.Sprintf(" ORDER BY id LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		p, e := scanPatient(rows)
		if e != nil {
			return result, e
		}
		result.Data = append(result.Data, p)
	}
	if err = rows.Err(); err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
