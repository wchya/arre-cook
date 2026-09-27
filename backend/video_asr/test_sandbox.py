"""Run in the Linux application image; no network or real credentials required."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


@unittest.skipUnless(sys.platform == "linux", "production sandbox requires Linux")
class SandboxIntegrationTests(unittest.TestCase):
    def test_runtime_cannot_read_or_modify_application_files_or_connect(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            job, model = root / "job", root / "model"
            job.mkdir()
            model.mkdir()
            for name in ("model.int8.onnx", "tokens.txt", "silero_vad.onnx"):
                (model / name).write_bytes(b"test model")
            secret = root / "application-config"
            secret.write_text("sandbox-test-canary")
            worker = r'''
import ctypes, importlib.util, json, os, pathlib, socket, subprocess, sys
module = pathlib.Path(sys.argv[1])
spec = importlib.util.spec_from_file_location("sandbox", module)
sandbox = importlib.util.module_from_spec(spec)
spec.loader.exec_module(sandbox)
job, model, secret = map(pathlib.Path, sys.argv[2:5])
assert secret.read_text() == "sandbox-test-canary"
abi = sandbox.enter(job, model, pathlib.Path("/usr/bin/ffmpeg"))
checks = {}
def blocked(name, action):
    try:
        action()
    except PermissionError:
        checks[name] = True
    else:
        checks[name] = False
blocked("read_config", lambda: secret.read_text())
blocked("write_config", lambda: secret.write_text("changed"))
blocked("readonly_truncate", lambda: os.close(os.open(secret, os.O_RDONLY | os.O_TRUNC)))
blocked("truncate", lambda: os.truncate(secret, 0))
blocked("chmod", lambda: secret.chmod(0o777))
blocked("parent_environment", lambda: pathlib.Path(f"/proc/{os.getppid()}/environ").read_bytes())
blocked("network_socket", lambda: socket.socket(socket.AF_INET, socket.SOCK_STREAM))
blocked("unix_socket", lambda: socket.socket(socket.AF_UNIX, socket.SOCK_STREAM))
blocked("shell_execution", lambda: subprocess.run(["/bin/sh", "-c", "exit 0"], check=True))
scratch = job / "scratch"
scratch.write_text("allowed")
checks["job_read_write"] = scratch.read_text() == "allowed"
scratch.unlink()
checks["model_read"] = (model / "tokens.txt").read_bytes() == b"test model"
blocked("model_write", lambda: (model / "tokens.txt").write_bytes(b"changed"))
print(json.dumps({"abi": abi, "checks": checks}), flush=True)
'''
            result = subprocess.run(
                [sys.executable, "-I", "-c", worker, str(Path(__file__).with_name("sandbox.py")),
                 str(job), str(model), str(secret)],
                capture_output=True, text=True, timeout=10,
                env={"PATH": "/usr/local/bin:/usr/bin:/bin", "LANG": "C.UTF-8", "TMPDIR": str(job)},
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            report = json.loads(result.stdout)
            self.assertGreaterEqual(report["abi"], 1)
            self.assertTrue(all(report["checks"].values()), report)
            self.assertEqual(secret.read_text(), "sandbox-test-canary")
            self.assertNotEqual(secret.stat().st_mode & 0o777, 0o777)


if __name__ == "__main__":
    unittest.main()
