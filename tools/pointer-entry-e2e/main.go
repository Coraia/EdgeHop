// Real Linux client → uinput → Hyprland E2E; no fake input device.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Coraia/EdgeHop/internal/protocol"
	"github.com/Coraia/EdgeHop/internal/secureconn"
)

type sample struct {
	MS int64   `json:"ms"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
}
type result struct {
	Name    string   `json:"name"`
	Passed  bool     `json:"passed"`
	Errors  []string `json:"errors,omitempty"`
	Samples []sample `json:"samples"`
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func hypr(args ...string) string {
	out, err := exec.Command("hyprctl", append([]string{"-i", "0"}, args...)...).CombinedOutput()
	must(err)
	return strings.TrimSpace(string(out))
}
func position() (float64, float64) {
	p := strings.Split(hypr("cursorpos"), ",")
	if len(p) != 2 {
		panic("invalid cursorpos")
	}
	x, err := strconv.ParseFloat(strings.TrimSpace(p[0]), 64)
	must(err)
	y, err := strconv.ParseFloat(strings.TrimSpace(p[1]), 64)
	must(err)
	return x, y
}
func warp(x, y float64) {
	out := hypr("dispatch", fmt.Sprintf("hl.dsp.cursor.move({ x = %.0f, y = %.0f })", x, y))
	if out != "ok" {
		out = hypr("dispatch", "movecursor", fmt.Sprintf("%.0f %.0f", x, y))
	}
	if out != "ok" {
		panic(out)
	}
}

func main() {
	if !runMain() {
		os.Exit(1)
	}
}

func runMain() bool {
	client := flag.String("client", "", "Linux client binary")
	output := flag.String("output", "", "new evidence directory")
	repro := flag.Bool("repro-only", false, "only reproduce fast right-side entry")
	flag.Parse()
	if *client == "" || *output == "" {
		panic("-client and -output required")
	}
	must(os.Mkdir(*output, 0700))
	originalX, originalY := position()
	defer warp(originalX, originalY)
	tmp, err := os.MkdirTemp("", "edgehop-pointer-e2e-")
	must(err)
	defer os.RemoveAll(tmp)
	secret, _, err := secureconn.LoadOrCreateSecret(filepath.Join(tmp, "pairing.key"))
	must(err)
	auth, err := secureconn.New(secret)
	must(err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	must(err)
	defer listener.Close()
	log, err := os.Create(filepath.Join(*output, "client.log"))
	must(err)
	defer log.Close()
	path, err := filepath.Abs(*client)
	must(err)
	cmd := exec.Command(path, "-server", listener.Addr().String(), "-pairing-file", filepath.Join(tmp, "pairing.key"), "-device", "edgehop-pointer-e2e", "-clip-interval", "1h")
	cmd.Stdout, cmd.Stderr = log, log
	must(cmd.Start())
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	listener.(*net.TCPListener).SetDeadline(time.Now().Add(10 * time.Second))
	raw, err := listener.Accept()
	must(err)
	conn, err := auth.Accept(raw)
	must(err)
	defer conn.Close()
	r := bufio.NewReader(conn)
	hello, err := protocol.ReadFrame(r)
	must(err)
	if hello.Type != protocol.MsgHello {
		panic("missing hello")
	}
	screen, err := protocol.ReadFrame(r)
	must(err)
	w, h, err := protocol.DecodeScreen(screen.Payload)
	must(err)
	if w < 700 || h < 500 {
		panic("E2E needs a real monitor >= 700x500")
	}
	send := func(typ byte, payload []byte) {
		must(conn.SetWriteDeadline(time.Now().Add(time.Second)))
		must(protocol.WriteFrame(conn, typ, payload))
	}
	send(protocol.MsgHello, []byte(protocol.Version))
	send(protocol.MsgScreen, protocol.EncodeScreen(1600, 1000))
	go func() {
		for {
			if _, err := protocol.ReadFrame(r); err != nil {
				return
			}
		}
	}() // Discard clipboard; never save its payload.
	data, err := os.ReadFile(path)
	must(err)
	metadata := map[string]any{"client_sha256": fmt.Sprintf("%x", sha256.Sum256(data)), "hyprland": hypr("version"), "width": w, "height": h, "path": "TLS frames → production Linux client → uinput → real Hyprland cursorpos", "physical_trackpad": "not tested"}
	b, err := json.MarshalIndent(metadata, "", "  ")
	must(err)
	must(os.WriteFile(filepath.Join(*output, "environment.json"), b, 0600))
	var results []result
	back := func() {
		send(protocol.MsgSwitch, protocol.EncodeSwitch(protocol.Switch{Direction: protocol.SwitchBack}))
		time.Sleep(30 * time.Millisecond)
	}
	run := func(name string, edge protocol.Edge, moving bool, macY int32) {
		back()
		send(protocol.MsgClientEdge, protocol.EncodeClientEdge(edge))
		warp(float64(w)/2, float64(h)/2)
		send(protocol.MsgSwitch, protocol.EncodeSwitch(protocol.Switch{Direction: protocol.SwitchRemote, Y: macY, HasY: true}))
		started := time.Now()
		res := result{Name: name, Passed: true}
		fail := func(message string) { res.Passed = false; res.Errors = append(res.Errors, message) }
		direction := 1.0
		parkX := 24.0
		if edge == protocol.EdgeRight {
			direction = -1
			parkX = float64(w) - 24
		}
		// Start incoming movement immediately after the switch; this overlaps the old parking loop.
		done := make(chan struct{})
		go func() {
			defer close(done)
			if !moving {
				return
			}
			time.Sleep(10 * time.Millisecond)
			for i := 0; i < 25; i++ {
				send(protocol.MsgMouseMove, protocol.EncodeMouseMove(int16(direction*16), 0))
				time.Sleep(8 * time.Millisecond)
			}
		}()
		for time.Since(started) < 1100*time.Millisecond {
			x, y := position()
			res.Samples = append(res.Samples, sample{time.Since(started).Milliseconds(), x, y})
			time.Sleep(10 * time.Millisecond)
		}
		<-done
		last := res.Samples[len(res.Samples)-1]
		if moving {
			for i := 1; i < len(res.Samples); i++ {
				a, b := res.Samples[i-1], res.Samples[i]
				if a.MS >= 80 && (b.X-a.X)*direction < -3 {
					fail(fmt.Sprintf("reverse jump %.0f → %.0f at %dms", a.X, b.X, b.MS))
				}
				if a.MS >= 450 && (b.X-a.X > 3 || a.X-b.X > 3) {
					fail(fmt.Sprintf("cursor still moving after input stopped at %dms", b.MS))
				}
			}
			if (last.X-parkX)*direction < 200 {
				fail("fast input was lost or pulled back toward entry")
			}
		} else {
			if last.X-parkX > 2 || parkX-last.X > 2 {
				fail(fmt.Sprintf("entry x=%.0f expected %.0f", last.X, parkX))
			}
			wantY := float64(macY) * float64(h) / 1000
			if last.Y-wantY > 2 || wantY-last.Y > 2 {
				fail(fmt.Sprintf("entry y=%.0f expected %.0f", last.Y, wantY))
			}
		}
		results = append(results, res)
		fmt.Printf("%s passed=%t failures=%d\n", name, res.Passed, len(res.Errors))
	}
	run("right-fast-entry", protocol.EdgeRight, true, 250)
	if !*repro {
		run("left-fast-entry", protocol.EdgeLeft, true, 750)
		run("right-stationary-ratio", protocol.EdgeRight, false, 250)
		run("left-stationary-ratio", protocol.EdgeLeft, false, 750)
		back()
		for i := 0; i < 4; i++ {
			send(protocol.MsgSwitch, protocol.EncodeSwitch(protocol.Switch{Direction: protocol.SwitchRemote, Y: 500, HasY: true}))
			time.Sleep(20 * time.Millisecond)
			back()
		}
		run("rapid-reentry-fast", protocol.EdgeLeft, true, 500)
	}
	back()
	b, err = json.MarshalIndent(results, "", "  ")
	must(err)
	must(os.WriteFile(filepath.Join(*output, "results.json"), b, 0600))
	for _, res := range results {
		if !res.Passed {
			return false
		}
	}
	return true
}
