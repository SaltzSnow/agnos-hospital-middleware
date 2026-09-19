CREATE TABLE hospitals (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL
);
CREATE TABLE staff (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    hospital_id BIGINT NOT NULL REFERENCES hospitals(id),
    username TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    UNIQUE (hospital_id, username)
);
CREATE TABLE patients (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    hospital_id BIGINT NOT NULL REFERENCES hospitals(id),
    patient_hn TEXT NOT NULL CHECK (btrim(patient_hn) <> ''),
    first_name_th TEXT, middle_name_th TEXT, last_name_th TEXT,
    first_name_en TEXT, middle_name_en TEXT, last_name_en TEXT,
    date_of_birth DATE,
    national_id TEXT, passport_id TEXT, phone_number TEXT, email TEXT,
    gender TEXT CHECK (gender IN ('M', 'F')),
    UNIQUE (hospital_id, patient_hn)
);
CREATE INDEX patients_hospital_id_id_idx ON patients(hospital_id, id);
CREATE INDEX patients_hospital_national_id_idx ON patients(hospital_id, national_id);
CREATE INDEX patients_hospital_passport_id_idx ON patients(hospital_id, passport_id);
-- Hospital configuration is required for staff provisioning, independently of demo data.
INSERT INTO hospitals (code, name) VALUES ('A', 'Hospital A'), ('B', 'Hospital B');
