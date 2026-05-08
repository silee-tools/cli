package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/silee-tools/mydesk/internal/config"
	"github.com/silee-tools/mydesk/internal/web"
)

func RunWeb(opts GlobalOpts, port int, noOpen bool, version string) error {
	cfg, err := config.Load(opts.ConfigDir)
	if err != nil {
		return err
	}

	addr := fmt.Sprintf("localhost:%d", port)
	s := web.New(cfg.ConfigDir, version)

	if !noOpen {
		go openBrowser("http://" + addr)
	}

	return s.Start(addr)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		fmt.Fprintf(os.Stderr, "Auto-open not supported on %s. Open manually: %s\n", runtime.GOOS, url)
		return
	}
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Could not open browser. Open manually: %s\n", url)
	}
}
