package main

import "testing"

func TestLoadConfigAppliesDefaultsAndEnvironmentOverrides(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("GOLB_HTTP_PORT", "env:9090")
	t.Setenv("GOLB_BLOG_TITLE", "Test Blog")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Port != "env:9090" {
		t.Fatalf("HTTP.Port = %q", cfg.HTTP.Port)
	}
	if cfg.HTTP.ReadTimeoutInSeconds != 10 || cfg.HTTP.IdleTimeoutInSeconds != 60 {
		t.Fatalf("defaults were not applied: %#v", cfg.HTTP)
	}
	if cfg.Blog.Title != "Test Blog" {
		t.Fatalf("Blog.Title = %q", cfg.Blog.Title)
	}
}
