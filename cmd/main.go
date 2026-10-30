package main

import (
	"fmt"
	"os"

	"github.com/chens-hi/shortcuts/internal/agent"
	"github.com/chens-hi/shortcuts/internal/app"
	"github.com/chens-hi/shortcuts/internal/config"
	"github.com/chens-hi/shortcuts/internal/deployer"
	"github.com/chens-hi/shortcuts/internal/logger"
	"github.com/chens-hi/shortcuts/internal/repository"
)

func main() {
	command := "server"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	var err error
	switch command {
	case "server":
		err = app.NewServer().Run()
	case "agent":
		err = agent.NewServer().Run()
	case "cleanup":
		err = runCleanup()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", command)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

// runCleanup uninstalls every child app recorded in the data file. It is invoked
// by shortcuts/cmd/uninstall_init when the user picks "remove" in the uninstall
// wizard, replacing the node scripts the shell used to call which were never
// shipped in the package. It stops and uninstalls only; it never edits the data
// file, so a reinstall can restore the shortcut list.
func runCleanup() error {
	cfg := config.New().WithResolvedPaths()
	log := logger.New()
	repo := repository.NewJSONRepository(cfg.DataFile)
	entries, err := repo.Load()
	if err != nil {
		return err
	}

	appCenter := deployer.NewManager(deployer.Settings{
		MainAppname:  cfg.MainAppname,
		AgentSocket:  cfg.AgentSocket,
		AppCenterCLI: cfg.AppCenterCLI,
		Timeout:      cfg.CLITimeout,
	})

	total, removed, failed := 0, 0, 0
	for _, entry := range entries {
		child := entry.ChildAppname(cfg.MainAppname)
		if child == "" {
			continue
		}
		total++
		_ = appCenter.Stop(child)
		result := appCenter.Uninstall(child)
		log.Info("cleanup uninstall child", "child", child, "ok", result.OK, "code", result.Code, "via", result.Via,
			"output", logger.SanitizeOutput(result.Stdout+"\n"+result.Stderr, 2000))
		if result.OK {
			removed++
		} else {
			failed++
		}
	}
	log.Info("cleanup finished", "total", total, "removed", removed, "failed", failed)
	fmt.Printf("cleanup: %d child app(s), %d removed, %d failed\n", total, removed, failed)
	return nil
}
