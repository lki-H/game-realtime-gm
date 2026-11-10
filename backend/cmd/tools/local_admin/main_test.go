package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdministratorPasswordRejectsBcryptByteOverflowAndMultiline(t *testing.T) {
	for _, value := range []string{"short", strings.Repeat("中", 25), "valid-long-password\ninjection", strings.Repeat("x", 1025)} {
		path := filepath.Join(t.TempDir(), "password")
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := password(path); err == nil {
			t.Fatal("invalid password accepted")
		}
	}
	path := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(path, []byte("a-unique-local-password\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if value, err := password(path); err != nil || string(value) != "a-unique-local-password" {
		t.Fatal("valid bounded password rejected")
	}
}
