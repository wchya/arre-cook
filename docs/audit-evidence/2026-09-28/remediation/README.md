# 修复后的验证记录

本目录记录修复后正确行为的测试结果，不覆盖上一级目录中断言缺陷存在的历史证据。

- [verification.json](verification.json)：Go 1.25.14 全仓 race 测试逐项状态、包状态和其他检查摘要。
- [govulncheck.txt](govulncheck.txt)：发布工具链下的源码扫描输出。
- [govulncheck-linux-binary.txt](govulncheck-linux-binary.txt)：Go 1.25.14 编译的 Linux amd64 二进制扫描输出。
- [mysql.txt](mysql.txt)：临时 MySQL 8.0.45、独立连接池的六组回归输出。只使用虚构账号与数据。
- [concurrent-migrations.txt](concurrent-migrations.txt)：两个真实应用迁移进程同时启动、每个数据库池限制为 1 的最终结果。
- [npm-audit.txt](npm-audit.txt)：官方 npm registry 审计结果。

回归源码分布在 `backend/internal/routes/audit_regression_test.go`、`backend/internal/services/audit_{hardening,mysql}_test.go`、`backend/internal/database/*_test.go`、`backend/internal/storage/inventory_test.go`、`backend/internal/mailer/mailer_test.go`、`backend/internal/resourcebudget/heavy_test.go`、`backend/cmd/dbmigrate/main_test.go` 和 `scripts/tests/session-security.test.cjs`。

MySQL 运行在临时目录、仅开放 Unix socket、关闭 MySQL X 和 TCP，buffer pool 64 MiB；从未连接生产数据库。`TestAuditMySQL` 只接受 `ARRE_AUDIT_MYSQL_SOCKET`，父目录名必须以 `arre-audit-mysql-` 开头，测试创建独立 `arre_audit_*` schema。没有该变量时明确跳过。

双进程迁移验证使用编译后的 server，均设置 `DB_MIGRATE_ONLY=true`、`DB_MAX_OPEN_CONNS=1`，同连独立空 schema；最终两个进程退出码均为 0，1 位管理员、100 道种子菜，`uploads_inventory_v1=done`。完整运行后关闭临时 mysqld。

生产发布、Linux 组合负载与 iOS 真机验收状态以 [修复记录](../../../audit-remediation-2026-09-28.md) 为准。
