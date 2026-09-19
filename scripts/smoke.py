#!/usr/bin/env python3
"""Exercise the synthetic demo through Nginx using only the Python standard library."""
import json
import os
from pathlib import Path
import secrets
import sys
import time
import urllib.error
import urllib.request
import uuid


class SmokeFailure(Exception):
    """A deliberately safe error that contains no response payloads or secrets."""


def configuration():
    values = {}
    env_path = Path(__file__).resolve().parent.parent / ".env"
    if env_path.exists():
        for line in env_path.read_text().splitlines():
            line = line.strip()
            if line and not line.startswith("#") and "=" in line:
                key, value = line.split("=", 1)
                values[key.strip()] = value.strip().strip("\"'")
    values.update(os.environ)
    if not values.get("REGISTRATION_KEY"):
        raise SmokeFailure("Registration key missing; run make setup first")
    return values


def run():
    config = configuration()
    base_url = config.get("BASE_URL", "http://127.0.0.1:" + config.get("HTTP_PORT", "8080")).rstrip("/")
    count = 0

    def passed(name):
        nonlocal count
        count += 1
        print(f"PASS {count:02d}: {name}", flush=True)

    def request(path, body=None, token=None, registration=False, expected=200):
        headers = {"Content-Type": "application/json"}
        if token:
            headers["Authorization"] = "Bearer " + token
        if registration:
            headers["X-Registration-Key"] = config["REGISTRATION_KEY"]
        raw = None if body is None else json.dumps(body).encode()
        req = urllib.request.Request(base_url + path, data=raw, headers=headers)
        try:
            response = urllib.request.urlopen(req, timeout=5)
        except urllib.error.HTTPError as error:
            response = error
        except (OSError, urllib.error.URLError):
            raise SmokeFailure("Cannot connect to the demonstration API") from None
        with response:
            if response.status != expected:
                raise SmokeFailure(f"Expected HTTP {expected}, received HTTP {response.status}")
            try:
                return json.loads(response.read())
            except (ValueError, OSError):
                raise SmokeFailure("API did not return valid JSON") from None

    deadline = time.monotonic() + 45
    while True:
        try:
            request("/readyz")
            break
        except SmokeFailure:
            if time.monotonic() >= deadline:
                raise SmokeFailure("API readiness timed out after 45 seconds") from None
            time.sleep(0.5)
    passed("API readiness through Nginx")

    username = "smoke_" + uuid.uuid4().hex
    password = secrets.token_urlsafe(24)
    credentials = {"username": username, "password": password, "hospital": "A"}
    request("/staff/create", credentials, expected=403)
    passed("Provisioning key required")
    tokens = {}
    for hospital in ("A", "B"):
        credentials = {"username": username, "password": password, "hospital": hospital}
        staff = request("/staff/create", credentials, registration=True, expected=201)
        if "password" in staff or "password_hash" in staff:
            raise SmokeFailure("Staff response exposed a password field")
        passed(f"Staff creation in hospital {hospital}")
        request("/staff/create", credentials, registration=True, expected=409)
        passed(f"Duplicate staff rejected in hospital {hospital}")
        request("/staff/login", {**credentials, "password": "incorrect-password"}, expected=401)
        passed(f"Incorrect password rejected in hospital {hospital}")
        login = request("/staff/login", credentials)
        token = login.get("access_token")
        if not isinstance(token, str) or not token:
            raise SmokeFailure("Login response omitted the access token")
        tokens[hospital] = token
        passed(f"Staff login in hospital {hospital}")
    request("/patient/search", {}, expected=401)
    passed("Patient authentication required")

    def search(hospital, filters, expected_hns=None, source=None):
        result = request("/patient/search", filters, token=tokens[hospital])
        data = result.get("data")
        if not isinstance(data, list) or not isinstance(result.get("total"), int):
            raise SmokeFailure("Patient response has an invalid result shape")
        if any(not str(row.get("patient_hn", "")).startswith(hospital) for row in data):
            raise SmokeFailure("Patient result escaped its hospital scope")
        if expected_hns is not None:
            if [row.get("patient_hn") for row in data] != expected_hns or result["total"] != len(expected_hns):
                raise SmokeFailure("Patient result did not match the synthetic fixture")
        if source is not None and result.get("source") != source:
            raise SmokeFailure("Patient result used an unexpected data source")
        return result

    for hospital in ("A", "B"):
        search(hospital, {"national_id": "0000000000001"}, [hospital + "001"], "his")
        passed(f"Shared national ID remains scoped to hospital {hospital}")
        search(hospital, {"passport_id": "DEMO-SHARED"}, [hospital + "001"], "his")
        passed(f"Shared passport remains scoped to hospital {hospital}")
        search(hospital, {}, source="local")
        passed(f"Unfiltered local listing remains scoped to hospital {hospital}")
    search("A", {"passport_id": "HIS-B900"}, [], "his")
    passed("Other hospital passport is not imported")

    filters_to_check = [
        ("English partial first name", {"first_name": "omch"}),
        ("Thai partial first name", {"first_name": "สม"}),
        ("English middle name", {"middle_name": "emo"}),
        ("Thai middle name", {"middle_name": "ทด"}),
        ("English partial last name", {"last_name": "aide"}),
        ("Thai partial last name", {"last_name": "ใจ"}),
        ("Birth date", {"date_of_birth": "1990-01-15"}),
        ("Exact phone", {"phone_number": "0800000001"}),
        ("Exact email", {"email": "a001@example.test"}),
        ("Combined AND filters", {"first_name": "Somchai", "last_name": "Jaidee", "date_of_birth": "1990-01-15"}),
    ]
    for name, filters in filters_to_check:
        search("A", filters, ["A001"], "local")
        passed(name)
    search("A", {"first_name": "Somchai", "date_of_birth": "1985-02-20"}, [], "local")
    passed("Contradictory AND filters return no patients")
    search("A", {"first_name": "%_\\"}, ["A002"], "local")
    passed("LIKE wildcard and escape characters match literally")
    search("A", {"first_name": "' OR 1=1 --"}, [], "local")
    passed("SQL injection text is treated as data")
    search("A", {"national_id": "0000000000001", "passport_id": "HIS-A900"}, [], "his")
    passed("Mismatched national and passport IDs return no patients")

    invalid_filters = [
        ("Null filter", {"national_id": None}),
        ("Empty filter", {"email": ""}),
        ("Whitespace filter", {"first_name": "  "}),
        ("Unknown hospital filter", {"hospital": "B"}),
        ("Unknown hospital ID filter", {"hospital_id": 2}),
        ("Malformed date", {"date_of_birth": "2023-02-29"}),
        ("Oversized page size", {"page_size": 101}),
        ("Invalid page", {"page": 0}),
        ("Oversized page", {"page": 1000001}),
    ]
    for name, filters in invalid_filters:
        request("/patient/search", filters, token=tokens["A"], expected=400)
        passed(name + " rejected")

    for hospital in ("A", "B"):
        imported = search(hospital, {"national_id": "0000000000900"}, [hospital + "900"], "his")
        passed(f"HIS-only record imported into hospital {hospital}")
        repeated = search(hospital, {"national_id": "0000000000900"}, [hospital + "900"], "his")
        if imported["data"][0]["id"] != repeated["data"][0]["id"]:
            raise SmokeFailure("Repeated HIS import changed the stored patient ID")
        search(hospital, {"first_name": "Imported"}, [hospital + "900"], "local")
        passed(f"Repeated import remains unique and locally scoped in hospital {hospital}")
        first = search(hospital, {"page": 1, "page_size": 1}, source="local")
        second = search(hospital, {"page": 1, "page_size": 1}, source="local")
        if first != second or len(first["data"]) != 1:
            raise SmokeFailure("Patient pagination was not stable")
        beyond = search(hospital, {"page": 1000000, "page_size": 1}, source="local")
        if beyond["data"] != [] or beyond["total"] != first["total"]:
            raise SmokeFailure("Pagination beyond the last page returned unexpected results")
        passed(f"Stable pagination and empty last page in hospital {hospital}")
    print(f"All {count} smoke checks passed.")


if __name__ == "__main__":
    try:
        run()
    except SmokeFailure as error:
        print(f"FAIL: {error}", file=sys.stderr)
        sys.exit(1)
    except Exception:
        # Suppress unexpected tracebacks: they may contain request bodies or tokens.
        print("FAIL: Unexpected smoke-check error", file=sys.stderr)
        sys.exit(1)
