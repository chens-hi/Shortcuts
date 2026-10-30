package service

import (
	"testing"

	"github.com/chens-hi/shortcuts/internal/deployer"
	"github.com/chens-hi/shortcuts/internal/model"
)

func TestChildAtCurrentVersion(t *testing.T) {
	s := &Service{}
	current := model.Entry{Version: "1.0.2", DeployedVersion: "1.0.2"}
	child := "shortcuts-moviepilot"

	// A child that is not installed is never current.
	if version, ok := s.childAtCurrentVersion(map[string]deployer.InstalledApp{}, child, current); ok {
		t.Fatalf("childAtCurrentVersion = (%q, true) for a missing child, want false", version)
	}

	// Installed but recorded as an older version is outdated.
	if _, ok := s.childAtCurrentVersion(map[string]deployer.InstalledApp{
		child: {AppName: child},
	}, child, model.Entry{Version: "1.0.2", DeployedVersion: "1.0.1"}); ok {
		t.Fatal("childAtCurrentVersion reported up to date for an outdated child")
	}

	// appcenter reports the same version as recorded.
	version, ok := s.childAtCurrentVersion(map[string]deployer.InstalledApp{
		child: {AppName: child, Version: "1.0.2"},
	}, child, current)
	if !ok || version != "1.0.2" {
		t.Fatalf("childAtCurrentVersion = (%q, %v), want (\"1.0.2\", true)", version, ok)
	}

	// appcenter reports nothing; the locally recorded version decides.
	version, ok = s.childAtCurrentVersion(map[string]deployer.InstalledApp{
		child: {AppName: child},
	}, child, current)
	if !ok || version != "1.0.2" {
		t.Fatalf("childAtCurrentVersion = (%q, %v), want (\"1.0.2\", true) from DeployedVersion", version, ok)
	}

	// appcenter reports a different version than recorded: not current.
	if _, ok := s.childAtCurrentVersion(map[string]deployer.InstalledApp{
		child: {AppName: child, Version: "1.0.3"},
	}, child, current); ok {
		t.Fatal("childAtCurrentVersion reported up to date when appcenter reports a different version")
	}

	// Never deployed: nothing recorded, so there is nothing to compare against.
	if _, ok := s.childAtCurrentVersion(map[string]deployer.InstalledApp{
		child: {AppName: child},
	}, child, model.Entry{Version: "1.0.0"}); ok {
		t.Fatal("childAtCurrentVersion reported up to date for an entry that was never deployed")
	}
}

func TestClaimDeployRejectsConcurrentDeployOfSameChild(t *testing.T) {
	s := &Service{childBusy: map[string]bool{}}

	if !s.claimChild("shortcuts-moviepilot") {
		t.Fatal("expected the first claim to succeed")
	}
	if s.claimChild("shortcuts-moviepilot") {
		t.Fatal("expected a second claim of the same child to be rejected")
	}
	if !s.claimChild("shortcuts-baidu") {
		t.Fatal("expected a different child to be claimable in parallel")
	}
	if len(s.childBusy) != 2 {
		t.Fatalf("childBusy = %v, want 2 entries", s.childBusy)
	}
}

func TestClaimDeploySurvivesNilMapAndAllowsReclaim(t *testing.T) {
	s := &Service{}

	if !s.claimChild("shortcuts-moviepilot") {
		t.Fatal("expected a claim on a nil map to succeed")
	}
	s.releaseChild("shortcuts-moviepilot")
	if s.childBusy["shortcuts-moviepilot"] {
		t.Fatal("expected the child to be released")
	}
	if !s.claimChild("shortcuts-moviepilot") {
		t.Fatal("expected the child to be reclaimable after release")
	}
	s.releaseChild("shortcuts-moviepilot")
	if len(s.childBusy) != 0 {
		t.Fatalf("childBusy = %v, want empty", s.childBusy)
	}
}

func TestClaimDeployRejectsEmptyAppName(t *testing.T) {
	s := &Service{childBusy: map[string]bool{}}

	if s.claimChild("") {
		t.Fatal("expected an empty child appname to be rejected")
	}
	if len(s.childBusy) != 0 {
		t.Fatalf("childBusy = %v, want empty", s.childBusy)
	}
}

func TestReleaseDeployIsIdempotent(t *testing.T) {
	s := &Service{}

	s.releaseChild("never-claimed")
	if len(s.childBusy) != 0 {
		t.Fatalf("childBusy = %v, want empty after releasing an unknown child", s.childBusy)
	}
}

func TestClaimDeployGrantsExactlyOnePerChildUnderConcurrency(t *testing.T) {
	s := &Service{}
	const workers = 50
	granted := make(chan bool, workers)
	for i := 0; i < workers; i++ {
		go func() { granted <- s.claimChild("shortcuts-moviepilot") }()
	}
	wins := 0
	for i := 0; i < workers; i++ {
		if <-granted {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("granted claims = %d, want exactly 1", wins)
	}
}
