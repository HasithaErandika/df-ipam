package sqlbundle

import (
	"os"
	"testing"
)

func TestSetupFileIsCurrent(t *testing.T) {
	want, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../sql/supabase-setup.sql")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatal("api/sql/supabase-setup.sql is out of date: run `make sql`")
	}
}
