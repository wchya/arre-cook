#!/usr/bin/env python3
"""Create guest-only Douyin credentials on an operator's desktop, never the server.

Requires websockets (see requirements-douyin.txt) and Chrome or Edge. Uses a fresh
profile, blocks media, never accesses the user's browser profile, and never logs
cookie values. Platform login or an interactive challenge ends the operation.
"""

import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time
import urllib.request


GUEST_NAMES = set("""
ttwid s_v_web_id passport_csrf_token passport_csrf_token_default
__ac_nonce __ac_signature __ac_referer msToken odin_tt UIFID UIFID_TEMP
web_sign_token x-web-secsdk-uid fpk1 fpk2 biz_trace_id IsDouyinActive
enter_pc_once device_web_cpu_core device_web_memory_size dy_sheight dy_swidth
hevc_supported home_can_add_dy_2_desktop is_support_rtm_web_ts strategyABtestKey
stream_recommend_feed_params
""".split())
LOGIN_NAMES = {"sessionid", "sessionid_ss", "sid_tt", "sid_guard", "uid_tt", "uid_tt_ss"}
REQUIRED_NAMES = {"ttwid", "s_v_web_id"}


def guest_payload(cookies):
    scoped = [c for c in cookies if c.get("domain", "").lstrip(".") in {"douyin.com", "www.douyin.com"}]
    if any(c.get("name") in LOGIN_NAMES and c.get("value") for c in scoped):
        raise ValueError("检测到登录态，拒绝导出；请关闭窗口后重新使用独立游客浏览器")
    selected = {}
    for c in scoped:
        name, value = c.get("name"), c.get("value", "")
        if name not in GUEST_NAMES or not value or c.get("path") != "/":
            continue
        expires = c.get("expires", -1)
        if expires > 0 and expires <= time.time():
            continue
        if len(value) > 4096 or any(ord(ch) < 0x21 or ord(ch) >= 0x7f or ch in '\\";,' for ch in value):
            raise ValueError("平台返回了无法安全保存的游客凭据")
        selected[name] = {k: c[k] for k in ("name", "value", "domain", "path")}
        selected[name]["expires"] = expires
    if not REQUIRED_NAMES <= selected.keys():
        raise ValueError("尚未取得有效游客凭据，请稍后重新运行")
    return {"version": 1, "cookies": [selected[k] for k in sorted(selected)]}


def write_credentials(output, payload):
    data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
    if len(data) > 32 << 10:
        raise ValueError("游客凭据超过大小限制")
    temporary = None
    try:
        with tempfile.NamedTemporaryFile(dir=output.parent, prefix=".douyin-", delete=False) as f:
            temporary = Path(f.name)
            os.chmod(temporary, 0o600)
            f.write(data)
        os.replace(temporary, output)
    finally:
        if temporary:
            temporary.unlink(missing_ok=True)


def browser_path(explicit):
    candidates = [explicit] if explicit else [
        "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
        "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
    ]
    for root in (os.environ.get("PROGRAMFILES"), os.environ.get("PROGRAMFILES(X86)"), os.environ.get("LOCALAPPDATA")):
        if root and not explicit:
            candidates.extend(str(Path(root) / p) for p in ("Google/Chrome/Application/chrome.exe", "Microsoft/Edge/Application/msedge.exe"))
    if not explicit:
        candidates.extend(shutil.which(name) for name in ("google-chrome", "chromium", "microsoft-edge"))
    for candidate in candidates:
        if candidate and Path(candidate).is_file():
            return candidate
    raise ValueError("未找到 Chrome 或 Edge，请用 --browser 指定本机浏览器程序")


def local_json(port, path):
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    with opener.open(f"http://127.0.0.1:{port}/{path}", timeout=3) as response:
        return json.load(response)


class CDP:
    def __init__(self, socket):
        self.socket, self.seq = socket, 0

    def send(self, method, params=None):
        self.seq += 1
        self.socket.send(json.dumps({"id": self.seq, "method": method, "params": params or {}}))
        return self.seq

    def event(self, message):
        if message.get("method") != "Fetch.requestPaused":
            return
        p = message["params"]
        mime = next((h["value"].lower() for h in p.get("responseHeaders", []) if h["name"].lower() == "content-type"), "")
        blocked = p.get("resourceType") in {"Media", "Image"} or mime.startswith(("video/", "audio/")) or "mpegurl" in mime or "dash+xml" in mime
        params = {"requestId": p["requestId"]}
        if blocked:
            params["errorReason"] = "BlockedByClient"
        self.send("Fetch.failRequest" if blocked else "Fetch.continueRequest", params)

    def call(self, method, params=None, timeout=10):
        request_id = self.send(method, params)
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            message = json.loads(self.socket.recv(timeout=max(.1, deadline-time.monotonic())))
            if message.get("id") == request_id:
                if "error" in message:
                    raise ValueError("浏览器调试调用失败：" + method)
                return message.get("result", {})
            self.event(message)
        raise TimeoutError("浏览器响应超时")

    def pump(self, seconds):
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            try:
                self.event(json.loads(self.socket.recv(timeout=max(.05, deadline-time.monotonic()))))
            except TimeoutError:
                return


def collect(browser, output, timeout):
    from websockets.sync.client import connect

    output.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    # All browser and temporary files remain beside the requested output, never
    # in an OS-wide temp directory or the user's normal browser profile.
    with tempfile.TemporaryDirectory(prefix=".douyin-browser-", dir=output.parent) as work:
        profile = Path(work) / "profile"
        process, browser_ws = None, None
        try:
            process = subprocess.Popen([browser, "--user-data-dir="+str(profile), "--remote-debugging-address=127.0.0.1", "--remote-debugging-port=0", "--no-first-run", "--no-default-browser-check", "--disable-background-networking", "--disable-component-update", "--autoplay-policy=user-gesture-required", "about:blank"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            marker = profile / "DevToolsActivePort"
            deadline = time.monotonic() + 25
            while not marker.exists() and time.monotonic() < deadline:
                if process.poll() is not None:
                    raise ValueError("独立浏览器启动失败")
                time.sleep(.25)
            if not marker.exists():
                raise ValueError("独立浏览器启动超时")
            port = int(marker.read_text().splitlines()[0])
            browser_ws = local_json(port, "json/version")["webSocketDebuggerUrl"]
            page = next(p for p in local_json(port, "json/list") if p["type"] == "page")
            with connect(page["webSocketDebuggerUrl"], max_size=8<<20, open_timeout=5) as socket:
                cdp = CDP(socket)
                cdp.call("Network.enable")
                cdp.call("Network.setCacheDisabled", {"cacheDisabled": True})
                cdp.call("Network.setBlockedURLs", {"urls": ["*.douyinvod.com/*", "*.bytevod.com/*", "*.mp4*", "*.mp3*", "*.m4a*", "*.m3u8*", "*/aweme/v1/play/*"]})
                cdp.call("Fetch.enable", {"patterns": [{"urlPattern": "*", "requestStage": stage} for stage in ("Request", "Response")]})
                cdp.call("Page.enable")
                cdp.call("Page.navigate", {"url": "https://www.douyin.com/"})
                deadline, partial_since = time.monotonic()+timeout, None
                while time.monotonic() < deadline:
                    cdp.pump(2)
                    state = cdp.call("Runtime.evaluate", {"expression": "JSON.stringify({challenge:/拖动滑块|请完成验证|点击完成验证|安全验证/.test(document.body?.innerText||'')})", "returnByValue": True})
                    if json.loads(state.get("result", {}).get("value", "{}" )).get("challenge"):
                        raise ValueError("平台要求交互验证，已停止；不会自动处理验证码")
                    cookies = cdp.call("Network.getAllCookies").get("cookies", [])
                    scoped = [c for c in cookies if c.get("domain", "").lstrip(".") in {"douyin.com", "www.douyin.com"}]
                    names = {c["name"] for c in scoped if c.get("value")}
                    if LOGIN_NAMES & names:
                        raise ValueError("检测到登录态，已停止；此工具仅生成游客凭据")
                    if REQUIRED_NAMES <= names:
                        partial_since = partial_since or time.monotonic()
                        if "passport_csrf_token" in names or time.monotonic()-partial_since >= 6:
                            payload = guest_payload(scoped)
                            write_credentials(output, payload)
                            print(f"已保存 {len(payload['cookies'])} 项游客凭据到 {output}（权限 600），未下载视频。")
                            return
                raise ValueError("等待游客凭据超时，已停止；未更改原有凭据文件")
        finally:
            if browser_ws:
                try:
                    with connect(browser_ws, open_timeout=3, close_timeout=1) as socket:
                        CDP(socket).call("Browser.close", timeout=3)
                except Exception:
                    pass
            if process and process.poll() is None:
                process.terminate()
                try:
                    process.wait(timeout=8)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=3)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True, type=Path, help="凭据 JSON 输出路径，只供服务端读取")
    parser.add_argument("--browser", help="本机 Chrome 或 Edge 可执行文件")
    parser.add_argument("--timeout", type=int, default=90, choices=range(15, 181), metavar="15–180")
    args = parser.parse_args()
    os.umask(0o077)
    try:
        collect(browser_path(args.browser), args.output.absolute(), args.timeout)
    except KeyboardInterrupt:
        parser.exit(130, "已取消，独立浏览器已关闭。\n")
    except Exception as exc:
        # Never print raw upstream bodies, CDP messages, or cookie values.
        message = str(exc) if isinstance(exc, ValueError) else type(exc).__name__
        parser.exit(1, "游客凭据生成失败："+message+"\n")


if __name__ == "__main__":
    main()
