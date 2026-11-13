//go:build windows

package tray

import (
	"context"
	_ "embed"
	"errors"
	"log/slog"
	"path/filepath"

	"fyne.io/systray"
	"golang.org/x/sys/windows"
)

//go:embed assets/miyabi.ico
var icon []byte

const mutexName = `Local\miyabi-tray`

var instanceMutex windows.Handle

// Run shows the tray icon, runs serve in the background, and blocks until the
// user quits or serve stops. A second launch focuses the first instance.
func Run(ctx context.Context, cancel context.CancelFunc, opts Options, serve ServeFunc) error {
	owner, err := acquireInstance()
	if err != nil {
		return err
	}
	if !owner {
		openURL(opts.URL)
		return nil
	}
	defer releaseInstance()

	go func() {
		<-ctx.Done()
		systray.Quit()
	}()

	serveErr := make(chan error, 1)
	systray.Run(func() {
		systray.SetIcon(icon)
		systray.SetTooltip(opts.Title)

		openItem := systray.AddMenuItem("打开网页", opts.URL)
		dataItem := systray.AddMenuItem("打开数据目录", opts.DataDir)
		systray.AddSeparator()
		quitItem := systray.AddMenuItem("退出", "退出 "+opts.Title)

		systray.SetOnTapped(func() { openURL(opts.URL) })
		go func() {
			for range openItem.ClickedCh {
				openURL(opts.URL)
			}
		}()
		go func() {
			for range dataItem.ClickedCh {
				openPath(opts.DataDir)
			}
		}()
		go func() {
			<-quitItem.ClickedCh
			systray.Quit()
		}()

		go func() {
			serveErr <- serve(ctx)
			systray.Quit()
		}()
	}, func() {})

	cancel()
	return <-serveErr
}

func acquireInstance() (bool, error) {
	name, err := windows.UTF16PtrFromString(mutexName)
	if err != nil {
		return false, err
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		_ = windows.CloseHandle(handle)
		return false, nil
	}
	if err != nil {
		return false, err
	}
	instanceMutex = handle
	return true, nil
}

func releaseInstance() {
	if instanceMutex != 0 {
		_ = windows.CloseHandle(instanceMutex)
		instanceMutex = 0
	}
}

func openPath(path string) {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	openURL(path)
}

func openURL(target string) {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return
	}
	file, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return
	}
	if err := windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		slog.Warn("failed to open", "target", target, "error", err)
	}
}
