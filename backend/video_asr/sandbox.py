"""Linux restrictions inherited by FFmpeg and the inference runtime.

Landlock limits file access to the current job and the bundled runtime/model.
Seccomp denies networking, process inspection, and filesystem operations not
covered by older Landlock ABIs. Unsupported hosts fail closed.
"""
from __future__ import annotations

import ctypes
import errno
import os
from pathlib import Path
import platform


class SandboxUnavailable(Exception):
    pass


class Ruleset(ctypes.Structure):
    _fields_ = [("handled_access_fs", ctypes.c_uint64)]


class PathRule(ctypes.Structure):
    _pack_ = 1
    _fields_ = [("allowed_access", ctypes.c_uint64), ("parent_fd", ctypes.c_int32)]


class ArgCompare(ctypes.Structure):
    _fields_ = [("arg", ctypes.c_uint), ("op", ctypes.c_int),
                ("datum_a", ctypes.c_uint64), ("datum_b", ctypes.c_uint64)]


def _check(result: int) -> int:
    if result < 0:
        raise SandboxUnavailable("sandbox_unavailable")
    return result


def restrict_files(job: Path, model: Path, ffmpeg: Path) -> int:
    if platform.system() != "Linux" or platform.machine() not in ("x86_64", "aarch64"):
        raise SandboxUnavailable("sandbox_unavailable")
    libc = ctypes.CDLL(None, use_errno=True)
    libc.syscall.restype = ctypes.c_long
    abi = _check(libc.syscall(444, 0, 0, 1))
    handled = (1 << 13) - 1
    if abi >= 2:
        handled |= 1 << 13  # REFER: cross-directory links and renames
    if abi >= 3:
        handled |= 1 << 14  # TRUNCATE
    attributes = Ruleset(handled)
    ruleset = _check(libc.syscall(444, ctypes.byref(attributes), ctypes.sizeof(attributes), 0))
    read = (1 << 2) | (1 << 3)
    write = (1 << 1) | (1 << 5) | (1 << 8)
    if abi >= 3:
        write |= 1 << 14

    def allow(path: Path, access: int, required: bool = False) -> None:
        if not path.exists():
            if required:
                raise SandboxUnavailable("sandbox_unavailable")
            return
        # Landlock rules apply to inodes, including paths reached through aliases.
        path = path.resolve(strict=True)
        if not path.is_dir():
            access &= (1 << 0) | (1 << 1) | (1 << 2) | (1 << 14)
        fd = os.open(path, os.O_PATH | os.O_CLOEXEC)
        try:
            rule = PathRule(access, fd)
            _check(libc.syscall(445, ruleset, 1, ctypes.byref(rule), 0))
        finally:
            os.close(fd)

    try:
        allow(job, read | write, required=True)
        for name in ("model.int8.onnx", "tokens.txt", "silero_vad.onnx"):
            allow(model / name, read, required=True)
        for path in ("/usr/lib", "/usr/local/lib", "/lib", "/lib64", "/opt/video-asr/lib",
                     "/sys/devices/system/cpu"):
            allow(Path(path), read)
        for path in ("/etc/ld.so.cache", "/etc/localtime", "/dev/urandom", "/dev/random",
                     "/proc/cpuinfo", "/proc/meminfo", "/proc/self/status", "/proc/self/maps"):
            allow(Path(path), read)
        allow(Path("/dev/null"), (1 << 1) | (1 << 2), required=True)
        allow(ffmpeg, (1 << 0) | (1 << 2), required=True)
        # The ELF interpreter also needs execute access; library trees do not.
        for path in ("/lib64/ld-linux-x86-64.so.2", "/lib/ld-linux-aarch64.so.1"):
            allow(Path(path), (1 << 0) | (1 << 2))
        _check(libc.prctl(38, 1, 0, 0, 0))  # PR_SET_NO_NEW_PRIVS
        _check(libc.syscall(446, ruleset, 0))
    finally:
        os.close(ruleset)
    return abi


def restrict_syscalls() -> None:
    # Load before applying rules; no shell-based library discovery is needed.
    library = ctypes.CDLL("libseccomp.so.2", use_errno=True)
    library.seccomp_init.argtypes = [ctypes.c_uint32]
    library.seccomp_init.restype = ctypes.c_void_p
    library.seccomp_syscall_resolve_name.argtypes = [ctypes.c_char_p]
    library.seccomp_syscall_resolve_name.restype = ctypes.c_int
    library.seccomp_rule_add_array.argtypes = [ctypes.c_void_p, ctypes.c_uint32, ctypes.c_int,
                                             ctypes.c_uint, ctypes.POINTER(ArgCompare)]
    library.seccomp_rule_add_array.restype = ctypes.c_int
    library.seccomp_load.argtypes = [ctypes.c_void_p]
    library.seccomp_load.restype = ctypes.c_int
    library.seccomp_release.argtypes = [ctypes.c_void_p]
    context = library.seccomp_init(0x7FFF0000)  # SCMP_ACT_ALLOW
    if not context:
        raise SandboxUnavailable("sandbox_unavailable")

    def deny(name: str, compare: ArgCompare | None = None) -> None:
        number = library.seccomp_syscall_resolve_name(name.encode("ascii"))
        if number < 0:  # A syscall absent on this architecture cannot be invoked.
            return
        _check(library.seccomp_rule_add_array(context, 0x00050000 | errno.EPERM, number,
                                             int(compare is not None),
                                             ctypes.byref(compare) if compare is not None else None))

    try:
        for name in (
            "socket", "socketpair", "connect", "bind", "listen", "accept", "accept4",
            "sendto", "sendmsg", "sendmmsg", "recvmsg", "recvmmsg",
            "ptrace", "process_vm_readv", "process_vm_writev", "pidfd_getfd",
            "mount", "umount2", "pivot_root", "setns", "unshare", "bpf", "perf_event_open",
            "open_by_handle_at", "name_to_handle_at", "io_uring_setup", "memfd_create",
            "init_module", "finit_module", "delete_module", "keyctl", "add_key", "request_key",
            "chmod", "fchmod", "fchmodat", "fchmodat2", "chown", "lchown", "fchown", "fchownat",
            "utime", "utimes", "futimesat", "utimensat", "setxattr", "lsetxattr", "fsetxattr",
            "removexattr", "lremovexattr", "fremovexattr", "truncate", "ftruncate", "openat2",
        ):
            deny(name)
        # Landlock ABI 1/2 do not mediate truncation. Deny the unusual read-only
        # O_TRUNC form; writable opens still require Landlock WRITE_FILE access.
        for name, argument in (("open", 1), ("openat", 2)):
            deny(name, ArgCompare(argument, 7, os.O_ACCMODE | os.O_TRUNC, os.O_TRUNC))
        _check(library.seccomp_load(context))
    finally:
        library.seccomp_release(context)


def enter(job: Path, model: Path, ffmpeg: Path) -> int:
    try:
        abi = restrict_files(job, model, ffmpeg)
        restrict_syscalls()
        return abi
    except (OSError, ValueError) as error:
        raise SandboxUnavailable("sandbox_unavailable") from error
