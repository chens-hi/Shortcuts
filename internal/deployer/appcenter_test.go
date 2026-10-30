package deployer

import "testing"

func TestParseAppCenterList(t *testing.T) {
	output := "#  APP NAME  DISPLAY NAME  VERSION  STATUS  DEPENDENCY\n" +
		"│\n" +
		"1│shortcuts-moviepilot│MoviePilot│1.0.1│running│none\n" +
		"2│bad name here│Broken│1.0.0│stopped│none\n" +
		"3│shortcuts-baidu│百度一下│2.3.4│STOPPED│nodejs\n"

	apps := ParseAppCenterList(output)

	if len(apps) != 2 {
		t.Fatalf("parsed %d apps, want 2: %v", len(apps), apps)
	}
	got := apps["shortcuts-moviepilot"]
	if got.DisplayName != "MoviePilot" || got.Version != "1.0.1" || got.Status != "running" || got.Dependency != "none" {
		t.Fatalf("unexpected moviepilot app: %+v", got)
	}
	if got2 := apps["shortcuts-baidu"]; got2.Status != "stopped" || got2.DisplayName != "百度一下" {
		t.Fatalf("expected baidu to be parsed and lowercased: %+v", got2)
	}
	if _, ok := apps["bad name here"]; ok {
		t.Fatal("expected a malformed app name to be rejected")
	}
}

func TestParseAppCenterListAcceptsBoxDrawingsAndAsciiPipes(t *testing.T) {
	box := "1│shortcuts-a│A│1.0.0│running│none"
	ascii := "1|shortcuts-b|B|1.0.0|running|none"

	boxApps := ParseAppCenterList(box)
	asciiApps := ParseAppCenterList(ascii)

	if _, ok := boxApps["shortcuts-a"]; !ok {
		t.Fatalf("box-drawing row was not parsed: %v", boxApps)
	}
	if _, ok := asciiApps["shortcuts-b"]; !ok {
		t.Fatalf("ascii pipe row was not parsed: %v", asciiApps)
	}
}

func TestParseAppCenterListSkipsShortRowsAndHeaders(t *testing.T) {
	output := "│\nAPP NAME\n1│shortcuts-a\n2│shortcuts-a│A\n3│shortcuts-a│A│1.0.0│running│none\n"

	apps := ParseAppCenterList(output)
	if len(apps) != 1 {
		t.Fatalf("parsed %d apps, want 1: %v", len(apps), apps)
	}
}

func TestParseAppCenterListEmptyOutput(t *testing.T) {
	apps := ParseAppCenterList("")
	if len(apps) != 0 {
		t.Fatalf("parsed %d apps from empty output, want 0", len(apps))
	}
}

func TestFormatOutputTruncatesToTail(t *testing.T) {
	result := CommandResult{Stdout: "head ", Stderr: "tail"}
	if got := FormatOutput(result, 100); got != "head \ntail" {
		t.Fatalf("FormatOutput = %q, want stdout and stderr joined with a newline", got)
	}

	long := CommandResult{Stdout: "aaaaaaaaaa", Stderr: "bbbbbbbbbb"}
	if got := FormatOutput(long, 5); got != "bbbbb" {
		t.Fatalf("FormatOutput = %q, want the last %d bytes of stdout+stderr", got, 5)
	}
}
