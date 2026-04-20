package main

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"


	"github.com/nlink-jp/shell-agent/internal/config"
	"github.com/nlink-jp/shell-agent/internal/logger"
)

//go:embed sample_tools/*.sh
var sampleTools embed.FS

// installSampleTools copies sample tool scripts to the user's tools directory
// if it is empty or does not exist. Called once during startup.
func installSampleTools() {
	log := logger.New("tools")
	toolsDir := config.ExpandPath(config.DefaultConfig().Tools.ScriptDir)

	// Check if tools directory already has scripts
	entries, err := os.ReadDir(toolsDir)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				return // has at least one file — don't overwrite
			}
		}
	}

	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		log.Error("create tools dir: %v", err)
		return
	}

	scripts, err := fs.ReadDir(sampleTools, "sample_tools")
	if err != nil {
		log.Error("read sample tools: %v", err)
		return
	}

	for _, d := range scripts {
		if d.IsDir() {
			continue
		}
		data, err := sampleTools.ReadFile("sample_tools/" + d.Name())
		if err != nil {
			continue
		}
		dst := filepath.Join(toolsDir, d.Name())
		if err := os.WriteFile(dst, data, 0o755); err != nil {
			log.Error("install %s: %v", d.Name(), err)
			continue
		}
		log.Info("installed sample tool: %s", d.Name())
	}
}
