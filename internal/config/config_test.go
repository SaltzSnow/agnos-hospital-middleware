package config

import "testing"

func TestFromEnv(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "postgres://user:password@db:5432/agnos", "JWT_SECRET": "0123456789abcdef0123456789abcdef", "REGISTRATION_KEY": "abcdef0123456789abcdef0123456789"}
	for _, tc := range []struct {
		name, key, value string
		fail             bool
	}{
		{"defaults", "", "", false}, {"missing database", "DATABASE_URL", "", true},
		{"weak JWT", "JWT_SECRET", "short", true}, {"weak registration", "REGISTRATION_KEY", "short", true},
		{"example JWT", "JWT_SECRET", "replace-with-at-least-32-random-bytes", true},
		{"example registration", "REGISTRATION_KEY", "replace-with-random-provisioning-key", true},
		{"invalid port", "PORT", "65536", true}, {"timeout", "HIS_TIMEOUT", "0s", true},
		{"large timeout", "HIS_TIMEOUT", "1m", true}, {"HIS query", "HIS_A_BASE_URL", "https://his.test?token=bad", true},
		{"HIS credentials", "HIS_A_BASE_URL", "https://user:pass@his.test", true},
		{"HIS file URL", "HIS_A_BASE_URL", "file:///etc/passwd", true}, {"valid HIS", "HIS_A_BASE_URL", "http://mockhis:8080/a", false},
		{"same secrets", "REGISTRATION_KEY", base["JWT_SECRET"], true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			get := func(key string) string {
				if key == tc.key {
					return tc.value
				}
				return base[key]
			}
			c, err := FromEnv(get)
			if (err != nil) != tc.fail {
				t.Fatalf("error=%v want failure %v", err, tc.fail)
			}
			if err == nil && c.Port != "8080" {
				t.Fatalf("port=%s", c.Port)
			}
		})
	}
}
