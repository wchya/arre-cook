package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"ninimenu/internal/auth"
	"ninimenu/internal/config"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/resourcebudget"
	"ninimenu/internal/routes"
	"ninimenu/internal/services"
	"ninimenu/internal/storage"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	config.Load()
	if config.C.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	database.PasswordHasher = auth.HashPassword
	if err := database.Init(); err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	if os.Getenv("DB_MIGRATE_ONLY") == "true" {
		return
	}
	if err := services.AnnounceDeploymentWithNotes(config.C.AppVersion, config.C.ReleaseNotes); err != nil {
		log.Printf("发布版本更新站内信失败: %v", err)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	// 只信任本机与内网反向代理传来的 X-Forwarded-For，限流按真实 IP 生效
	if err := r.SetTrustedProxies(trustedProxies()); err != nil {
		log.Printf("设置可信代理失败: %v", err)
	}
	routes.Setup(r)

	requestCtx, cancelRequests := context.WithCancel(context.Background())
	defer cancelRequests()
	services.StartAchievementWorkers(requestCtx)
	srv := &http.Server{
		BaseContext:       func(net.Listener) context.Context { return requestCtx },
		Addr:              ":" + config.C.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		MaxHeaderBytes:    32 * 1024,
		IdleTimeout:       120 * time.Second,
		// 不设 WriteTimeout：AI 对话走 SSE 长连接
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go housekeeping(ctx)

	go func() {
		log.Printf("arre食谱推荐小助手 启动成功：http://localhost:%s（版本：%s，存储：%s，邮件：%v，微信登录：%v）",
			config.C.Port, config.C.AppVersion, storage.Backend(), config.C.SMTPEnabled(), config.C.WechatEnabled())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("启动失败: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("正在关闭服务…")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("优雅关闭超时: %v", err)
	}
	cancelRequests()
	_ = srv.Close()
	drainCtx, drainCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer drainCancel()
	_ = resourcebudget.Drain(drainCtx)
	if sqlDB, err := database.DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

func trustedProxies() []string {
	if v := strings.TrimSpace(os.Getenv("TRUSTED_PROXIES")); v != "" {
		return strings.Split(v, ",")
	}
	return []string{"127.0.0.1", "::1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}
}

// housekeeping 定期清理过期验证码、过期建议与过旧的审计/事件日志，控制 SQLite 体积。
func housekeeping(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	run := func() {
		now := time.Now()
		services.PurgeExpiredCodes(database.DB.WithContext(ctx))
		cleanupCtx, cleanupCancel := context.WithTimeout(ctx, 30*time.Second)
		if err := database.CleanupUploads(cleanupCtx, database.DB.WithContext(cleanupCtx)); err != nil {
			log.Printf("[uploads] cleanup deferred: %v", err)
		}
		cleanupCancel()
		database.DB.WithContext(ctx).Where("expires_at < ?", now).Delete(&models.RequestWindow{})
		database.DB.WithContext(ctx).Where("expires_at < ?", now.Add(-time.Hour)).Delete(&models.AssistantLease{})
		database.DB.WithContext(ctx).Where("usage_date < ?", now.AddDate(0, 0, -180).Format("2006-01-02")).Delete(&models.AssistantUsage{})
		database.DB.WithContext(ctx).Model(&models.AgentSuggestion{}).
			Where("status = ? AND expires_at IS NOT NULL AND expires_at < ?", "pending", now).
			Update("status", "expired")
		database.DB.WithContext(ctx).Where("created_at < ?", now.AddDate(0, 0, -180)).Delete(&models.AgentAuditLog{})
		// 每晚固定整点给清单里还有没买食材的用户发一条买菜提醒（当天已发过的会跳过）
		if now.Hour() == services.ShoppingReminderHour() {
			services.SendShoppingReminders(now)
		}
	}
	run()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
