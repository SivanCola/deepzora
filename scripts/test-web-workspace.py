#!/usr/bin/env python3
"""Exercise a packaged CLI's real Web UI with a local streaming model fixture.

Requires Python Playwright and Chromium. No real credentials or provider calls.
Usage: python scripts/test-web-workspace.py bin/deepzora [--browser-executable PATH]
"""
import argparse
import json
import os
from pathlib import Path
import queue
import re
import subprocess
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from playwright.sync_api import sync_playwright, expect


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--browser-executable")
    parser.add_argument("--evidence", type=Path)
    args = parser.parse_args()
    root = Path(tempfile.mkdtemp(prefix="deepzora-web-smoke-"))
    evidence = args.evidence or root / "evidence"
    evidence.mkdir(parents=True, exist_ok=True)
    workspace = root / "workspace"
    workspace.mkdir()
    subprocess.run(["git", "init", "-q", str(workspace)], check=True)
    before = "Before the Web edit.\n"
    (workspace / "web-proof.txt").write_text(before)
    subprocess.run(["git", "-C", str(workspace), "add", "web-proof.txt"], check=True)
    hooks = root / "no-hooks"
    hooks.mkdir()
    subprocess.run(["git", "-C", str(workspace), "-c", "user.name=Web Smoke", "-c", "user.email=web-smoke@example.invalid", "-c", f"core.hooksPath={hooks}", "-c", "commit.gpgSign=false", "commit", "-qm", "Web smoke fixture baseline"], check=True)
    home = root / "home"
    home.mkdir()
    requests = []
    stream_release = threading.Event()

    class Provider(BaseHTTPRequestHandler):
        def log_message(self, *_args):
            pass

        def do_GET(self):
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"data": [{"id": "test", "object": "model"}]}).encode())

        def do_POST(self):
            payload = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            requests.append(payload)
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.end_headers()

            def emit(delta, finish=None):
                self.wfile.write(("data: " + json.dumps({"choices": [{"index": 0, "delta": delta, "finish_reason": finish}]}) + "\n\n").encode())
                self.wfile.flush()

            messages = payload.get("messages", [])
            user_index = max((i for i, message in enumerate(messages) if message.get("role") == "user"), default=-1)
            user = json.dumps(messages[user_index].get("content", "")) if user_index >= 0 else ""
            results = [message for message in messages[user_index + 1:] if message.get("role") == "tool"]
            has_read = any(message.get("name") == "read_file" for message in results)
            has_write = any(message.get("name") == "write_file" for message in results)
            if "WEB_WRITE" in user and not has_read:
                # Existing-file writes must first observe the current file,
                # preserving the real engine's stale-write protection.
                emit({"tool_calls": [{"index": 0, "id": "web-read-1", "type": "function", "function": {"name": "read_file", "arguments": json.dumps({"path": str(workspace / "web-proof.txt")})}}]}, "tool_calls")
            elif "WEB_WRITE" in user and not has_write:
                emit({"tool_calls": [{"index": 0, "id": "web-write-1", "type": "function", "function": {"name": "write_file", "arguments": json.dumps({"path": str(workspace / "web-proof.txt"), "content": "Written by the shared Go engine.\n"})}}]}, "tool_calls")
            elif "WEB_WRITE" in user:
                emit({"content": "WEB_WRITE_DONE"}, "stop")
            else:
                emit({"content": "WEB_STREAM_"})
                # Keep the real turn active until the browser has observed the
                # first chunk and reloaded; runner speed must not decide this.
                if not stream_release.wait(timeout=90):
                    raise TimeoutError("Browser did not release the active stream")
                emit({"content": "DONE"}, "stop")
            self.wfile.write(b"data: [DONE]\n\n")
            self.wfile.flush()

    provider = ThreadingHTTPServer(("127.0.0.1", 0), Provider)
    threading.Thread(target=provider.serve_forever, daemon=True).start()
    (home / "config.toml").write_text(f'''default_model = "web-smoke/test"
[desktop]
language = "en"
check_updates = false
telemetry = false
metrics = false
default_tool_approval_mode = "workspace-write"
[permissions]
ask = ["write_file"]
[[providers]]
name = "web-smoke"
kind = "openai"
base_url = "http://127.0.0.1:{provider.server_port}/v1"
models = ["test"]
api_key_env = "DEEPZORA_WEB_SMOKE_KEY"
''')
    env = {**os.environ, "DEEPZORA_HOME": str(home), "DEEPZORA_WEB_SMOKE_KEY": "fixture-only", "XDG_CACHE_HOME": str(root / "cache"), "XDG_STATE_HOME": str(root / "state"), "XDG_CONFIG_HOME": str(root / "config")}
    command = [str(args.binary.resolve()), "web", "--no-open", "--dir", str(workspace)]
    process = subprocess.Popen(command, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    lines = queue.Queue()
    logs = []

    def read_log(stream):
        for line in stream:
            logs.append(line)
            lines.put(line)

    threading.Thread(target=read_log, args=(process.stdout,), daemon=True).start()
    errors = []
    frames = []
    console = []
    def wait_access_url():
        deadline = time.monotonic() + 90
        access_url = None
        while time.monotonic() < deadline:
            if process.poll() is not None:
                raise RuntimeError("CLI exited before startup: " + "".join(logs))
            try:
                line = lines.get(timeout=1)
            except queue.Empty:
                continue
            match = re.search(r"Deepzora Web: (http://127\.0\.0\.1:\d+/#token=[a-f0-9]+)", line)
            if match:
                access_url = match.group(1)
                break
        assert access_url, "CLI did not print a local authenticated URL"
        return access_url

    try:
        access_url = wait_access_url()
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(headless=True, executable_path=args.browser_executable)
            context = browser.new_context(viewport={"width": 1440, "height": 1000})
            page = context.new_page()
            page.on("pageerror", lambda error: errors.append(str(error)))
            page.on("console", lambda message: console.append(f"{message.type}: {message.text}"))
            def watch_socket(socket):
                socket.on("framesent", lambda data: frames.append({"direction": "sent", "data": data}))
                socket.on("framereceived", lambda data: frames.append({"direction": "received", "data": data}))
            page.on("websocket", watch_socket)
            page.goto(access_url)
            page.wait_for_load_state("networkidle")
            page.screenshot(path=str(evidence / "startup.png"), full_page=True)
            (evidence / "startup.txt").write_text(page.locator("body").inner_text())
            composer = page.locator('#composer-input')
            composer.wait_for(state="visible", timeout=60000)
            assert page.evaluate("document.documentElement.dataset.host") == "web"
            assert "#token=" not in page.url, "access credential remained in address bar"
            assert not page.locator("#web-connection-error").count()
            print("PASS packaged UI, real service, authentication, token removal", flush=True)

            composer.fill("WEB_STREAM test the shared engine")
            # The input is editable during cold workspace initialization, but
            # submission intentionally waits for the runtime/controller gate.
            expect(page.locator(".composer__btn--send")).to_be_enabled(timeout=60000)
            composer.press("Enter")
            page.get_by_text("WEB_STREAM_", exact=True).wait_for(timeout=60000)
            # Reload during the provider stream: browser lifetime cannot own execution.
            page.reload()
            page.wait_for_load_state("networkidle")
            stream_release.set()
            page.get_by_text("WEB_STREAM_DONE", exact=True).wait_for(timeout=60000)
            page.wait_for_timeout(1500)
            print("PASS streaming and reload recovery during an active turn", flush=True)
            composer = page.locator('#composer-input')
            composer.fill("WEB_WRITE edit web-proof.txt using write_file")
            expect(page.locator(".composer__btn--send")).to_be_enabled(timeout=60000)
            composer.press("Enter")
            page.screenshot(path=str(evidence / "write-pending.png"), full_page=True)
            # An explicit ask rule requires the same real approval flow as Desktop.
            approval = page.get_by_role("button", name="Confirm", exact=True)
            approval.wait_for(state="visible", timeout=60000)
            assert (workspace / "web-proof.txt").read_text() == before, "tool ran before approval"
            page.reload()
            page.wait_for_load_state("networkidle")
            approval = page.get_by_role("button", name="Confirm", exact=True)
            approval.wait_for(state="visible", timeout=60000)
            assert (workspace / "web-proof.txt").read_text() == before, "reload bypassed approval"
            approval.click()
            page.get_by_text("WEB_WRITE_DONE", exact=True).wait_for(timeout=60000)
            assert (workspace / "web-proof.txt").read_text() == "Written by the shared Go engine.\n"
            page.locator(".turn-files__list button").filter(has_text="web-proof.txt").click()
            diff = page.locator(".turn-file-diff")
            expect(diff).to_contain_text("Before the Web edit.", timeout=15000)
            expect(diff).to_contain_text("Written by the shared Go engine.", timeout=15000)
            page.screenshot(path=str(evidence / "completed.png"), full_page=True)
            print("PASS tool approval, real file mutation and workspace change inspection", flush=True)
            page.reload()
            page.wait_for_load_state("networkidle")
            page.get_by_text("WEB_WRITE_DONE", exact=True).wait_for(timeout=60000)
            assert len([request for request in requests if request.get("messages")]) >= 3
            assert not errors, errors
            print("PASS persisted session recovery without replaying a submission", flush=True)
            process.terminate()
            process.wait(timeout=25)
            assert process.returncode == 0, "CLI did not shut down cleanly"
            expect(page.locator("#web-connection-error")).to_be_visible(timeout=15000)
            print("PASS service shutdown surfaces connection loss", flush=True)
            request_count = len(requests)
            process = subprocess.Popen(command, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            threading.Thread(target=read_log, args=(process.stdout,), daemon=True).start()
            page.goto(wait_access_url())
            page.wait_for_load_state("networkidle")
            page.get_by_text("WEB_WRITE_DONE", exact=True).wait_for(timeout=60000)
            assert len(requests) == request_count, "restart replayed a model request"
            page.screenshot(path=str(evidence / "service-restarted.png"), full_page=True)
            print("PASS session recovery after a full service restart without replay", flush=True)
            browser.close()
    finally:
        stream_release.set()
        if process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=25)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
        provider.shutdown()
        # The access fragment is ephemeral; omit it from saved evidence.
        (evidence / "service.log").write_text(re.sub(r"#token=[a-f0-9]+", "#token=[redacted]", "".join(logs)))
        (evidence / "browser-errors.json").write_text(json.dumps(errors, indent=2))
        (evidence / "protocol.json").write_text(json.dumps(frames, indent=2))
        (evidence / "console.txt").write_text("\n".join(console))
        (evidence / "model-requests.json").write_text(json.dumps(requests, indent=2))
        print(f"Evidence: {evidence}", flush=True)


if __name__ == "__main__":
    main()
