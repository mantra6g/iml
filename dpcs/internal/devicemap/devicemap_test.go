package devicemap

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeDeviceMap(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "device-map.env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write device-map file: %v", err)
	}
	return path
}

func TestParseSingleAndMultipleTargets(t *testing.T) {
	path := writeDeviceMap(t, `
# comment line

1=bmv2-target
2 = lb-a , lb-b,lb-c
`)
	got, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	want := map[uint64][]string{
		1: {"bmv2-target"},
		2: {"lb-a", "lb-b", "lb-c"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse returned %v, want %v", got, want)
	}
}

func TestParseRejectsInvalidLines(t *testing.T) {
	testCases := []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "missing equals sign", content: "1 bmv2-target", wantErr: "malformed line 1"},
		{name: "non-numeric device id", content: "x=bmv2-target", wantErr: "invalid device_id on line 1"},
		{name: "empty target list", content: "1=", wantErr: "line 1: empty p4target name"},
		{name: "empty name between commas", content: "1=lb-a,,lb-b", wantErr: "line 1: empty p4target name"},
		{name: "trailing comma", content: "1=lb-a,", wantErr: "line 1: empty p4target name"},
		{name: "duplicate name in one line", content: "1=lb-a,lb-a", wantErr: `line 1: duplicate p4target name "lb-a"`},
		{name: "duplicate device id", content: "1=lb-a\n1=lb-b", wantErr: "duplicate device_id 1 on line 2"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := Parse(writeDeviceMap(t, testCase.content))
			if err == nil {
				t.Fatalf("Parse accepted %q", testCase.content)
			}
			if !strings.Contains(err.Error(), testCase.wantErr) {
				t.Errorf("Parse error %q does not contain %q", err, testCase.wantErr)
			}
		})
	}
}

func TestParseIfExistsReturnsEmptyMapForMissingFile(t *testing.T) {
	got, err := ParseIfExists(filepath.Join(t.TempDir(), "missing.env"))
	if err != nil {
		t.Fatalf("ParseIfExists failed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ParseIfExists returned %v, want empty map", got)
	}
}
