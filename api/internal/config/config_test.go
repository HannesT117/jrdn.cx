package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeEnv(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "portfolio-api.env")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMissingFileMeansDisabled(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.env"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled || cfg.Port != DefaultPort {
		t.Fatalf("got %+v", cfg)
	}
}

func TestLoadParsesAllKeys(t *testing.T) {
	cfg, err := Load(writeEnv(t, `
# comment
ENABLED=true
PORT=9000
OPPER_API_KEY=op-abc
HMAC_SECRET=s3cret
DAILY_CAP_PER_CLIENT=5
DAILY_CAP_GLOBAL=50
ALLOWED_ORIGINS=https://a.example, https://b.example
CLIENT_IP_HEADER=X-Forwarded-For
GARMIN_API_KEY=gk-123
OPPER_MANAGEMENT_KEY=op-mak-1
OPPER_PROJECT=Site
DAILY_BUDGET_USD=1.5
`))
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		Enabled: true, Port: 9000, OpperKey: "op-abc", HMACSecret: "s3cret",
		CapPerClient: 5, CapGlobal: 50, ClientIPHeader: "X-Forwarded-For",
		GarminAPIKey: "gk-123", ManagementKey: "op-mak-1", Project: "Site", DailyBudgetUSD: 1.5,
		Origins: []string{"https://a.example", "https://b.example"},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("got %+v", cfg)
	}
}

func TestEnabledWithoutSecretsIsAnError(t *testing.T) {
	if _, err := Load(writeEnv(t, "ENABLED=true\nOPPER_API_KEY=op-x\nHMAC_SECRET=x\nALLOWED_ORIGINS=https://a.example\n")); err == nil {
		t.Fatal("expected an error without OPPER_MANAGEMENT_KEY")
	}
	if _, err := Load(writeEnv(t, "ENABLED=true\nOPPER_API_KEY=op-x\nHMAC_SECRET=x\nOPPER_MANAGEMENT_KEY=op-mak-1\n")); err == nil {
		t.Fatal("expected an error without ALLOWED_ORIGINS")
	}
	if _, err := Load(writeEnv(t, "ENABLED=true\nHMAC_SECRET=x\nALLOWED_ORIGINS=https://a.example\n")); err == nil {
		t.Fatal("expected an error without OPPER_API_KEY")
	}
	if _, err := Load(writeEnv(t, "ENABLED=true\nOPPER_API_KEY=op-x\nALLOWED_ORIGINS=https://a.example\n")); err == nil {
		t.Fatal("expected an error without HMAC_SECRET")
	}
}

func TestMalformedValuesAreErrors(t *testing.T) {
	for _, body := range []string{"PORT=abc\n", "DAILY_CAP_GLOBAL=-1\n", "DAILY_BUDGET_USD=0\n", "DAILY_BUDGET_USD=two\n", "ENABLED=yes please\n", "NO_EQUALS_SIGN\n", "UNKNOWN_KEY=1\n"} {
		if _, err := Load(writeEnv(t, body)); err == nil {
			t.Errorf("expected an error for %q", body)
		}
	}
}

func TestLoadStripsMatchingQuotes(t *testing.T) {
	cfg, err := Load(writeEnv(t, `GARMIN_API_KEY="gk-1.2_3"
OPPER_API_KEY='op-abc'
HMAC_SECRET="half
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GarminAPIKey != "gk-1.2_3" || cfg.OpperKey != "op-abc" || cfg.HMACSecret != `"half` {
		t.Fatalf("got %+v", cfg)
	}
}

func TestBudgetDefaultsToTwoDollarsForThePortfolioProject(t *testing.T) {
	cfg := Defaults()
	if cfg.DailyBudgetUSD != 2 || cfg.Project != "Portfolio" {
		t.Fatalf("got %+v", cfg)
	}
}
