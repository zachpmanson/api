package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServeRequiresKeyBeforeCreatingDatabase(t *testing.T) {
	t.Setenv("PROD_BROKE_KEY", "")
	path := filepath.Join(t.TempDir(), "state.sqlite")
	if err := run([]string{"serve", "-db", path}); err == nil {
		t.Fatal("started without shared key")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("created database despite missing key", err)
	}
}

func TestImportCommandIsRepeatableWithoutKey(t *testing.T) {
	t.Setenv("PROD_BROKE_KEY", "")
	path := filepath.Join(t.TempDir(), "state.sqlite")
	args := []string{"import", "-db", path, "-file", "../../internal/tracker/testdata/history.json"}
	for i := 0; i < 2; i++ {
		if err := run(args); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCommandRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{{"import"}, {"unknown"}, {"serve", "-file", "unused"}, {"import", "-file", "/nonexistent/import.json"}} {
		if err := run(args); err == nil {
			t.Fatal("accepted invalid args", args)
		}
	}
}
