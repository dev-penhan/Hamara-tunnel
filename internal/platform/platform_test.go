package platform

import "testing"

func TestParseOSRelease(t *testing.T) {
	m := parseOSRelease("ID=ubuntu\nVERSION_ID=\"24.04\"\n# comment\n")
	if m["ID"] != "ubuntu" || m["VERSION_ID"] != "24.04" {
		t.Fatalf("unexpected values: %#v", m)
	}
}
