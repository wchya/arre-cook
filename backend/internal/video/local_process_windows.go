package video

import "os/exec"

// The bundled inference runtime is Linux-only; keep other application builds compatible.
func configureLocalProcess(cmd *exec.Cmd) {}
