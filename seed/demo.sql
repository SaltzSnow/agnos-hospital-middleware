-- Synthetic demonstration records only. Safe to apply repeatedly.
INSERT INTO patients (hospital_id, patient_hn, first_name_th, middle_name_th, last_name_th,
 first_name_en, middle_name_en, last_name_en, date_of_birth, national_id, passport_id,
 phone_number, email, gender)
SELECT id, 'A001', 'สมชาย', 'ทดสอบ', 'ใจดี', 'Somchai', 'Demo', 'Jaidee',
 '1990-01-15'::date, '0000000000001', 'DEMO-SHARED', '0800000001', 'a001@example.test', 'M'
FROM hospitals WHERE code = 'A'
ON CONFLICT (hospital_id, patient_hn) DO NOTHING;
INSERT INTO patients (hospital_id, patient_hn, first_name_th, first_name_en, last_name_en, national_id)
SELECT id, 'A002', 'ทดสอบ', E'Literal%_\\Name', 'Example', '0000000000002'
FROM hospitals WHERE code = 'A'
ON CONFLICT (hospital_id, patient_hn) DO NOTHING;
INSERT INTO patients (hospital_id, patient_hn, first_name_th, last_name_th, first_name_en,
 last_name_en, date_of_birth, national_id, passport_id, phone_number, email, gender)
SELECT id, 'B001', 'สมชาย', 'อื่น', 'Somchai', 'Other', '1985-02-20'::date,
 '0000000000001', 'DEMO-SHARED', '0800000002', 'b001@example.test', 'M'
FROM hospitals WHERE code = 'B'
ON CONFLICT (hospital_id, patient_hn) DO NOTHING;
