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

// AcquireInstance claims the single-instance slot for this user. owner is false
// when another instance already holds it; the caller should then open the
// existing UI and exit. Call it before any database or log work so a second
// launch never touches the shared state.
func AcquireInstance() (owner bool, release func(), err error) {
	name, err := windows.UTF16PtrFromString(mutexName)
	if err != nil {
		return false, nil, err
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		_ = windows.CloseHandle(handle)
		return false, func() {}, nil
	}
	if err != nil {
		return false, nil, err
	}
	instanceMutex = handle
	return true, func() {
		if instanceMutex != 0 {
			_ = windows.CloseHandle(instanceMutex)
			instanceMutex = 0
		}
	}, nil
}

// Open opens target with the OS default handler (best-effort).
func Open(target string) {
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

// Run shows the tray icon, runs serve in the background, and blocks until the
// user quits or serve stops. The caller holds the single-instance lock.
func Run(ctx context.Context, cancel context.CancelFunc, opts Options, serve ServeFunc) error {
	go func() {
		<-ctx.Done()
		systray.Quit()
	}()

	serveErr := make(chan error, 1)
	systray.Run(func() {
		systray.SetIcon(icon)
		systray.SetTooltip(opts.Title)

		openItem := systray.AddMenuItem("打开界面", opts.URL)
		dataItem := systray.AddMenuItem("打开数据目录", opts.DataDir)
		systray.AddSeparator()
		quitItem := systray.AddMenuItem("退出", "退出 "+opts.Title)

		systray.SetOnTapped(func() { Open(opts.URL) })
		go func() {
			for range openItem.ClickedCh {
				Open(opts.URL)
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

func openPath(path string) {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	Open(path)
}
