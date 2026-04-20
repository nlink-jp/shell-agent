package main

import (
	"context"
	"embed"
	"os"

	"github.com/nlink-jp/shell-agent/internal/analysis"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Subcommand routing: "analyze" runs background analysis mode
	if len(os.Args) > 1 && os.Args[1] == "analyze" {
		analysis.RunCLI(os.Args[2:])
		return
	}

	app := NewApp()

	err := wails.Run(&options.App{
		Title:            "Shell Agent",
		Width:            1024,
		Height:           768,
		EnableDefaultContextMenu: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		OnBeforeClose: func(ctx context.Context) (prevent bool) {
			if app.isProcessing() {
				dialog, err := wailsRuntime.MessageDialog(ctx, wailsRuntime.MessageDialogOptions{
					Type:          wailsRuntime.QuestionDialog,
					Title:         "Processing in progress",
					Message:       "An analysis or tool is currently running. Quit anyway? Results may be lost.",
					DefaultButton: "No",
					Buttons:       []string{"Yes", "No"},
				})
				if err != nil || dialog == "No" {
					return true // prevent close
				}
			}
			return false
		},
		Bind: []interface{}{
			app,
		},
		Mac: &mac.Options{
			TitleBar: &mac.TitleBar{
				TitlebarAppearsTransparent: true,
				HideTitle:                 true,
				FullSizeContent:           true,
			},
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
