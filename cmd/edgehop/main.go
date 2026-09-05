// EdgeHop is the macOS menu-bar front end for the input-sharing server. It runs
// the server
// in-process (so a single Accessibility grant covers everything), shows status
// in the menu bar, and needs no terminal.
//
// Package it as an .app bundle (see `make bundle`): an LSUIElement app that
// lives only in the menu bar.
package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/Coraia/EdgeHop/internal/secureconn"
	"github.com/Coraia/EdgeHop/internal/server"
	"github.com/getlantern/systray"
)

//go:embed assets/menubar.png
var menuIcon []byte

const (
	appLabel       = "io.coraia.edgehop"
	legacyAppLabel = "com.universalcontrol.app"
	tooltip        = "EdgeHop — Mac ⇄ Linux"
)

var logPath = filepath.Join(os.Getenv("HOME"), "Library", "Logs", "EdgeHop.log")

func appSupportDir() string {
	return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "edgehop")
}

func legacyAppSupportDir() string {
	return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "universal-control")
}

// configFilePath is an optional JSON config that overrides defaults so the app
// can be configured without launching from a terminal. Fields: listen, edge,
// connect.
func configFilePath() string {
	return filepath.Join(appSupportDir(), "config.json")
}

func pairingFilePath() string {
	return filepath.Join(appSupportDir(), "pairing.key")
}

type fileConfig struct {
	Listen     string `json:"listen"`
	Edge       string `json:"edge"`
	SwitchKeys []int  `json:"switchKeys"` // macOS keycodes to hold together to toggle remote (nil/empty = default Cmd+Shift+Space)
}

// defaultSwitchKeys is the built-in hotkey (⌘⇧Space) used to toggle remote
// control when none is configured, so there is always a reliable way back to
// the Mac even if edge detection does not fire.
var defaultSwitchKeys = []int{55, 56, 49} // Command, Shift, Space

func main() {
	var (
		listen = flag.String("listen", "0.0.0.0:24800", "TCP listen address")
		edge   = flag.String("edge", "right", "client edge: right or left")
	)
	set := map[string]bool{}
	flag.Parse()
	flag.Visit(func(f *flag.Flag) { set[f.Name] = true })

	// Log to file first so fatal migration errors are visible for a GUI launch.
	setupLogging()
	if err := migrateLegacyData(legacyAppSupportDir(), appSupportDir()); err != nil {
		log.Fatalf("migrate legacy data: %v", err)
	}

	// Apply optional config file for flags not given on the command line.
	switchKeys := defaultSwitchKeys
	if fc, err := readFileConfig(); err == nil {
		if !set["listen"] && fc.Listen != "" {
			*listen = fc.Listen
		}
		if !set["edge"] && (fc.Edge == "left" || fc.Edge == "right") {
			*edge = fc.Edge
		}
		if len(fc.SwitchKeys) > 0 {
			switchKeys = fc.SwitchKeys
		}
	}

	if bin, err := os.Executable(); err == nil {
		if err := migrateLegacyLoginItem(legacyLoginItemPath(), loginItemPath(), bin); err != nil {
			log.Printf("migrate login item: %v", err)
		}
	}
	secret, pairingCode, err := secureconn.LoadOrCreateSecret(pairingFilePath())
	if err != nil {
		log.Fatalf("pairing key: %v", err)
	}

	systray.Run(func() { onReady(*listen, *edge, switchKeys, secret, pairingCode) }, onExit)
}

func readFileConfig() (*fileConfig, error) {
	b, err := os.ReadFile(configFilePath())
	if err != nil {
		return nil, err
	}
	var fc fileConfig
	if err := json.Unmarshal(b, &fc); err != nil {
		return nil, err
	}
	return &fc, nil
}

func migrateLegacyData(legacyDir, destinationDir string) error {
	dirReady := false
	for _, name := range []string{"pairing.key", "config.json"} {
		source := filepath.Join(legacyDir, name)
		data, err := os.ReadFile(source)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !dirReady {
			if err := os.MkdirAll(destinationDir, 0o700); err != nil {
				return err
			}
			dirReady = true
		}
		// O_EXCL never overwrites an existing EdgeHop file and avoids the
		// stat-then-write race.
		f, err := os.OpenFile(filepath.Join(destinationDir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if _, err := f.Write(data); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}

func saveRemoteEdge(path, edge string) error {
	if edge != server.EdgeLeft && edge != server.EdgeRight {
		return errors.New("invalid remote edge")
	}
	var cfg fileConfig
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return err
		}
		if cfg.Edge == edge {
			return nil // no change: skip the rewrite
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	cfg.Edge = edge
	data, err = json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// onReady runs on the main thread once the tray icon is live.
func onReady(listen, edge string, switchKeys []int, pairingSecret []byte, pairingCode string) {
	systray.SetTemplateIcon(menuIcon, menuIcon)
	systray.SetTooltip(tooltip)

	serverItem := systray.AddMenuItem("服务器: 启动中…", "EdgeHop 服务状态")
	clientItem := systray.AddMenuItem("客户端: 未连接", "Linux 客户端连接状态")
	modeItem := systray.AddMenuItem("模式: local", "当前控制模式")
	serverItem.Disable()
	clientItem.Disable()
	modeItem.Disable()

	systray.AddSeparator()
	openAccessItem := systray.AddMenuItem("打开辅助功能设置", "首次使用需在此授权")
	openLogItem := systray.AddMenuItem("打开日志", "查看运行日志")
	copyPairingItem := systray.AddMenuItem("复制配对码", "复制 Linux 客户端安装所需的配对码")
	loginItem := systray.AddMenuItemCheckbox("登录时自动启动", "登录后自动运行本应用", loginItemEnabled())

	systray.AddSeparator()
	layoutItem := systray.AddMenuItem("设备布局", "选择客户端位于这台 Mac 的哪一侧")
	clientLeftItem := layoutItem.AddSubMenuItemCheckbox("客户端在 Mac 左侧", "从 Mac 左边缘切换到客户端", edge == server.EdgeLeft)
	clientRightItem := layoutItem.AddSubMenuItemCheckbox("客户端在 Mac 右侧", "从 Mac 右边缘切换到客户端", edge == server.EdgeRight)

	systray.AddSeparator()
	quitItem := systray.AddMenuItem("退出", "退出 EdgeHop")

	edgeUpdates := make(chan string, 1)

	// Action handlers.
	go func() {
		for range openAccessItem.ClickedCh {
			_ = exec.Command("open", "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility").Start()
		}
	}()
	go func() {
		for range openLogItem.ClickedCh {
			_ = exec.Command("open", logPath).Start()
		}
	}()
	go func() {
		for range copyPairingItem.ClickedCh {
			cmd := exec.Command("/usr/bin/pbcopy")
			cmd.Stdin = strings.NewReader(pairingCode)
			if err := cmd.Run(); err != nil {
				log.Printf("copy pairing code: %v", err)
			}
		}
	}()
	go func() {
		for range loginItem.ClickedCh {
			if loginItem.Checked() {
				if err := disableLoginItem(); err != nil {
					log.Printf("disable login item: %v", err)
					continue
				}
				loginItem.Uncheck()
			} else {
				if err := enableLoginItem(); err != nil {
					log.Printf("enable login item: %v", err)
					continue
				}
				loginItem.Check()
			}
		}
	}()
	go func() {
		current := edge
		for {
			var selected string
			select {
			case <-clientLeftItem.ClickedCh:
				selected = server.EdgeLeft
			case <-clientRightItem.ClickedCh:
				selected = server.EdgeRight
			}
			if selected == current {
				continue
			}
			if err := saveRemoteEdge(configFilePath(), selected); err != nil {
				log.Printf("save device layout: %v", err)
				continue
			}
			current = selected
			if selected == server.EdgeLeft {
				clientLeftItem.Check()
				clientRightItem.Uncheck()
			} else {
				clientLeftItem.Uncheck()
				clientRightItem.Check()
			}
			// Coalesce: keep only the latest pending layout update.
			select {
			case edgeUpdates <- selected:
			default:
				<-edgeUpdates
				edgeUpdates <- selected
			}
		}
	}()
	go func() {
		<-quitItem.ClickedCh
		systray.Quit()
	}()

	// Start the server engine in-process.
	cfg := server.DefaultConfig()
	cfg.ListenAddr = listen
	cfg.RemoteEdge = edge
	cfg.RemoteEdgeUpdates = edgeUpdates
	cfg.PairingSecret = pairingSecret
	cfg.SwitchKeys = switchKeys
	go func() {
		if err := server.RunWithStatus(cfg, func(s server.Status) {
			updateStatus(serverItem, clientItem, modeItem, s)
		}); err != nil {
			log.Printf("server error: %v", err)
			serverItem.SetTitle("服务器: 启动失败")
		}
	}()
}

func updateStatus(serverItem, clientItem, modeItem *systray.MenuItem, s server.Status) {
	if s.ServerRunning {
		serverItem.SetTitle("服务器: 运行中")
	} else if s.LastError != "" {
		serverItem.SetTitle("服务器: " + brief(s.LastError))
	}
	if s.ClientConnected {
		clientItem.SetTitle("客户端: 已连接")
	} else {
		clientItem.SetTitle("客户端: 未连接")
	}
	modeItem.SetTitle("模式: " + s.Mode)
	if s.LastError != "" {
		modeItem.SetTooltip(s.LastError)
	} else {
		modeItem.SetTooltip("当前控制模式")
	}
}

func brief(err string) string {
	if len(err) > 40 {
		return err[:40] + "…"
	}
	return err
}

// --- logging ---------------------------------------------------------------

func setupLogging() {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		log.Printf("cannot create log dir: %v", err)
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Printf("cannot open log file: %v", err)
		return
	}
	log.SetOutput(io.MultiWriter(f, os.Stderr))
	log.Printf("=== EdgeHop started %s ===", time.Now().Format(time.RFC3339))
}

// --- login item (LaunchAgent) ---------------------------------------------

func loginItemPath() string {
	return filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", appLabel+".plist")
}

func legacyLoginItemPath() string {
	return filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", legacyAppLabel+".plist")
}

func loginItemEnabled() bool {
	_, err := os.Stat(loginItemPath())
	return err == nil
}

var loginPlistTpl = template.Must(template.New("la").Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>{{.Label}}</string>
	<key>ProgramArguments</key>
	<array>
		<string>{{.Binary}}</string>
	</array>
	<key>RunAtLoad</key><true/>
	<key>ProcessType</key><string>Interactive</string>
</dict>
</plist>
`))

func enableLoginItem() error {
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	return writeLoginItem(loginItemPath(), bin)
}

func migrateLegacyLoginItem(legacyPath, destinationPath, binary string) error {
	if _, err := os.Stat(legacyPath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := os.Stat(destinationPath); os.IsNotExist(err) {
		if err := writeLoginItem(destinationPath, binary); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return os.Remove(legacyPath)
}

func writeLoginItem(path, binary string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return loginPlistTpl.Execute(f, struct{ Label, Binary string }{appLabel, binary})
}

func disableLoginItem() error {
	err := os.Remove(loginItemPath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func onExit() {
	log.Printf("=== EdgeHop exited ===")
}
