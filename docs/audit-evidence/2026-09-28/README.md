# 2026-09-28 审查证据

对应 [完整审查报告](../../audit-report-arre-cook-2026-09-28.md)，核心代码基线 0872efc，收尾版本 81861e4。

这些用例**断言当前坏行为存在**。测试通过是复现成功，不是修复成功；修复之后应改为断言正确业务行为。没有向生产发送故障流量。

## 文件

| 文件 | 用途 |
|---|---|
| routes-reproductions.go.txt | 9 个 Go 路由/服务复现，含 SQL 写失败与确定性交错 |
| client-reproductions.cjs | 2 个真实客户端模块 + JSDOM/受控 HTTP adapter 的复现 |
| reproductions.log | 筛去初始化噪声后的实际复现输出 |
| govulncheck-server.log | 原始 server 源码扫描结果，包含需按线上配置排除的告警 |
| go-advisories.json | 本轮从 vuln.go.dev 获取的相关公告原文与版本区间 |

Go 用例通过 -overlay 加入测试编译，没有写入 backend 源文件。确定性交错通过 GORM callback 将“另一个操作已完成”的时点固定在目标写入之前，不依赖线程调度碰运气；尚未用真实 MySQL 双连接压力验证。

前端用例加载当前 TS 模块；HTTP adapter 是受控替身，不访问线上。多标签页用独立 store 和共享 storage 模拟，并实际派发 storage 事件。后续应补真实浏览器双页面 E2E。

## 重新运行

从仓库根目录执行下列 Python。要求本地已有 Go、Node 和 frontend/node_modules。数据库强制选择临时 SQLite，测试由项目 testutil 创建/删除；只会在系统临时目录写 overlay 文件。不要将测试环境指向生产数据库。

~~~python
import json
import os
from pathlib import Path
import subprocess
import tempfile

root = Path.cwd()
evidence = root / "docs/audit-evidence/2026-09-28"
assert (root / "backend/go.mod").is_file(), "请在仓库根目录运行"

with tempfile.TemporaryDirectory(prefix="arre-cook-cr-repro-") as tmp:
    overlay = Path(tmp) / "overlay.json"
    overlay.write_text(json.dumps({
        "Replace": {
            str(root / "backend/internal/routes/cr_audit_test.go"):
                str(evidence / "routes-reproductions.go.txt")
        }
    }))
    env = dict(os.environ)
    env["DB_DRIVER"] = "sqlite"
    subprocess.run([
        "go", "test", "-race", "-overlay=" + str(overlay),
        "./internal/routes", "-run", "^TestCR", "-count=1", "-v"
    ], cwd=root / "backend", env=env, check=True)

subprocess.run([
    "node", str(evidence / "client-reproductions.cjs")
], cwd=root, check=True)
~~~

## 结果映射

| 用例 | 对应 Finding |
|---|---|
| TestCRCloneBypassesQuota | F02 |
| TestCRSettingsAdmissionAndMenuAmplification | F03、F04 |
| TestCRFailedSecurityUpdatesReportSuccess | F06 |
| TestCRMenuCacheSurvivesDishEdit | F11 |
| TestCRShoppingFailureLeavesMealCommitted | F09 |
| TestCRCanceledRequestStillReadsDatabase | F05 |
| TestCRTransferFamilyCanRaceWithLeave | F10 |
| TestCRPasswordWriteCanRestoreRevokedGeneration | F06 |
| TestCRSuggestionAcceptSwallowsMealFailure | F09 |
| Web 跨标签页 / fragment | F07 / F08 |

## 扫描解释

govulncheck 日志的标准库是**本地 Go 1.26.5**。线上二进制为 **Go 1.25.14**，相关标准库漏洞在 1.25.13 已修复。HTTP/3、32 位 WebP、白名单之外的 TIFF 等告警不能未经入口核验就宣传为线上可利用。

线上实际链接 x/image v0.41.0，允许上传 WebP，GO-2026-6222 因而被报告为确认的暴露路径。未在任何线上服务中运行恶意图片、真实转写压测或数据库故障注入。
