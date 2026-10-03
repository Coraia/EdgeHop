#!/usr/bin/env python3
"""Collect real-device input-guard evidence without recording input contents.

This observer does not operate the UI or claim human checks have passed.
See docs/input-guard-validation.md for failure modes and acceptance steps.
"""
import argparse
import ctypes
import hashlib
import json
import os
from pathlib import Path
import signal
import subprocess
import time
from datetime import datetime


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--seconds", type=int, default=180)
    parser.add_argument("--ssh-host", default="omarchy")
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=False)
    (args.output / "observer.pid").write_text(str(os.getpid()) + "\n")
    stop = False

    def finish(signum, frame):
        nonlocal stop
        stop = True

    signal.signal(signal.SIGUSR1, finish)
    cg = ctypes.CDLL("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
    cf = ctypes.CDLL("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation")
    carbon = ctypes.CDLL("/System/Library/Frameworks/Carbon.framework/Carbon")
    cg.CGSessionCopyCurrentDictionary.restype = ctypes.c_void_p
    cf.CFRelease.argtypes = [ctypes.c_void_p]
    cf.CFStringCreateWithCString.argtypes = [ctypes.c_void_p, ctypes.c_char_p, ctypes.c_uint32]
    cf.CFStringCreateWithCString.restype = ctypes.c_void_p
    cf.CFDictionaryGetValue.argtypes = [ctypes.c_void_p, ctypes.c_void_p]
    cf.CFDictionaryGetValue.restype = ctypes.c_void_p
    cf.CFGetTypeID.argtypes = [ctypes.c_void_p]
    cf.CFGetTypeID.restype = ctypes.c_ulong
    cf.CFBooleanGetTypeID.restype = ctypes.c_ulong
    cf.CFBooleanGetValue.argtypes = [ctypes.c_void_p]
    cf.CFBooleanGetValue.restype = ctypes.c_bool
    cf.CFNumberGetValue.argtypes = [ctypes.c_void_p, ctypes.c_int, ctypes.c_void_p]
    cf.CFNumberGetValue.restype = ctypes.c_bool
    carbon.IsSecureEventInputEnabled.restype = ctypes.c_bool
    cg.CGCursorIsVisible.restype = ctypes.c_int32
    keys = {k: cf.CFStringCreateWithCString(None, k.encode(), 0x08000100) for k in
            ("CGSSessionScreenIsLocked", "kCGSSessionOnConsoleKey", "kCGSessionLoginDoneKey")}

    def value(session, key):
        v = cf.CFDictionaryGetValue(session, keys[key]) if session else None
        if not v:
            return None
        if cf.CFGetTypeID(v) == cf.CFBooleanGetTypeID():
            return bool(cf.CFBooleanGetValue(v))
        n = ctypes.c_int()
        return bool(n.value) if cf.CFNumberGetValue(v, 9, ctypes.byref(n)) else None

    started = datetime.now().astimezone()
    mac_log = Path.home() / "Library/Logs/EdgeHop.log"
    offset = mac_log.stat().st_size
    with (args.output / "observations.jsonl").open("w") as out:
        deadline = time.monotonic() + args.seconds
        previous = None
        while not stop and time.monotonic() < deadline:
            session = cg.CGSessionCopyCurrentDictionary()
            state = {"session_available": bool(session),
                     "screen_locked": value(session, "CGSSessionScreenIsLocked"),
                     "on_console": value(session, "kCGSSessionOnConsoleKey"),
                     "login_done": value(session, "kCGSessionLoginDoneKey"),
                     "secure_input": bool(carbon.IsSecureEventInputEnabled()),
                     "cursor_visible": cg.CGCursorIsVisible()}
            if session:
                cf.CFRelease(session)
            if state != previous:
                out.write(json.dumps({"at": datetime.now().astimezone().isoformat(), **state}) + "\n")
                out.flush()
                previous = state
            time.sleep(0.1)
    for key in keys.values():
        cf.CFRelease(key)
    patterns = ("mode:", "input sharing paused:", "input sharing available",
                "cursor visibility after hide:", "remote control active", "control returned to local",
                "waiting for Accessibility permission")
    with mac_log.open("rb") as f:
        f.seek(offset)
        lines = f.read().decode(errors="replace").splitlines()
    (args.output / "mac-app.txt").write_text("\n".join(l for l in lines if any(p in l for p in patterns)) + "\n")
    start = started.strftime("%Y-%m-%d %H:%M:%S")
    end = datetime.now().astimezone().strftime("%Y-%m-%d %H:%M:%S")
    remote = f'journalctl -u edgehop-client --since "{start}" --until "{end}" --no-pager'
    result = subprocess.run(["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=6", args.ssh_host, remote], capture_output=True, text=True, timeout=20)
    lines = [l for l in result.stdout.splitlines() if "remote control:" in l or "connection error:" in l]
    (args.output / "linux-client.txt").write_text("\n".join(lines) + "\n")
    (args.output / "receipt.json").write_text(json.dumps({"start": started.isoformat(), "end": end,
        "seconds": args.seconds, "ssh_returncode": result.returncode, "ssh_stderr": result.stderr,
        "human_observations": "pending; observer does not verify screen pixels or shortcut effects"}, indent=2) + "\n")
    files = sorted(p for p in args.output.iterdir() if p.is_file())
    (args.output / "SHA256SUMS").write_text("".join(hashlib.sha256(p.read_bytes()).hexdigest() + "  " + p.name + "\n" for p in files))
    print(args.output)


if __name__ == "__main__":
    main()
