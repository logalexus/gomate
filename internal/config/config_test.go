package config

import "testing"

func TestPublicListenDefaultAndExplicitOverride(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/gomate")
	t.Setenv("WEB_PASSWORD", "random-test-password")
	t.Setenv("LISTEN", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "0.0.0.0:65000" {
		t.Fatal(cfg.Listen)
	}
	t.Setenv("LISTEN", "127.0.0.1:65000")
	cfg, err = Load()
	if err != nil || cfg.Listen != "127.0.0.1:65000" {
		t.Fatalf("override: %v %v", cfg.Listen, err)
	}
	t.Setenv("WEB_PASSWORD", "replace-with-a-different-random-password")
	if _, err = Load(); err == nil {
		t.Fatal("example credentials accepted")
	}
}
