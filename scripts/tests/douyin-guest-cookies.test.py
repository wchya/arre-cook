import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest


spec = importlib.util.spec_from_file_location("douyin_guest", Path(__file__).parents[1] / "douyin-guest-cookies.py")
helper = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helper)


class GuestCookieTests(unittest.TestCase):
    def cookie(self, name, **extra):
        return {"name": name, "value": "guest-value", "domain": ".douyin.com", "path": "/", "expires": -1, **extra}

    def test_only_scoped_guest_credentials_are_exported(self):
        cookies = [self.cookie("ttwid"), self.cookie("s_v_web_id"), self.cookie("unknown"), self.cookie("msToken", domain=".evil.invalid"), self.cookie("__ac_nonce", expires=1)]
        payload = helper.guest_payload(cookies)
        self.assertEqual({c["name"] for c in payload["cookies"]}, {"ttwid", "s_v_web_id"})

    def test_login_and_header_injection_are_rejected(self):
        base = [self.cookie("ttwid"), self.cookie("s_v_web_id")]
        for extra in [self.cookie("sessionid"), self.cookie("msToken", value="evil\r\nHeader: value")]:
            with self.assertRaises(ValueError):
                helper.guest_payload(base + [extra])
        with self.assertRaises(ValueError):
            helper.guest_payload([self.cookie("ttwid", expires=1)])

    def test_atomic_replacement_keeps_private_permissions(self):
        root = Path(os.environ.get("TMPDIR", Path.home() / ".cache"))
        root.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(dir=root) as work:
            output = Path(work) / "guest.json"
            output.write_text("old")
            payload = helper.guest_payload([self.cookie("ttwid"), self.cookie("s_v_web_id")])
            helper.write_credentials(output, payload)
            self.assertEqual(json.loads(output.read_text()), payload)
            if os.name != "nt":
                self.assertEqual(output.stat().st_mode & 0o777, 0o600)
            self.assertEqual([p.name for p in Path(work).iterdir()], ["guest.json"])


if __name__ == "__main__":
    unittest.main()
