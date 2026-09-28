package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ninimenu/internal/models"
	"ninimenu/internal/storage"
	"reflect"
	"strings"
	"time"
)

const uploadLock = "upload-objects-and-references"
const UserUploadBytes int64 = 256 << 20
const SiteUploadBytes int64 = 5 << 30
const UserUploadObjects = 1000
const SiteUploadObjects = 50000

var ErrUploadQuota = errors.New("图片存储额度已用完，请清理未使用图片")
var ErrUploadReferenced = errors.New("图片仍被菜谱、头像或记录使用，请先移除引用")
var ErrUploadGone = errors.New("图片已过期或正在回收，请重新上传")

var uploadSources = map[string][]string{
	"dishes": {"image_url", "images", "steps"}, "users": {"avatar"},
	"meal_records": {"photo"}, "user_day_ratings": {"photos"},
}

// RegisterUploadReferences covers every write path (including batch/map updates)
// in the same GORM transaction as its source row. GC uses the same lock, so it
// cannot delete an object between validating and committing a new reference.
func RegisterUploadReferences(db *gorm.DB) error {
	before := func(tx *gorm.DB) {
		if tx.Error != nil || uploadSources[tx.Statement.Table] == nil {
			return
		}
		clean := tx.Session(&gorm.Session{NewDB: true})
		if err := LockKey(clean, uploadLock); err != nil {
			tx.AddError(err)
			return
		}
		if tx.Statement.SQL.Len() != 0 {
			return
		}
		var ids []uint
		q := clean.Model(tx.Statement.Model).Unscoped()
		if where, ok := tx.Statement.Clauses["WHERE"]; ok {
			q = q.Clauses(where.Expression)
		}
		if err := q.Pluck("id", &ids).Error; err != nil {
			tx.AddError(err)
			return
		}
		tx.Statement.Settings.Store("upload:ids", ids)
	}
	after := func(tx *gorm.DB) {
		if tx.Error != nil || uploadSources[tx.Statement.Table] == nil {
			return
		}
		var ids []uint
		if v, ok := tx.Statement.Settings.Load("upload:ids"); ok {
			ids = v.([]uint)
		}
		if len(ids) == 0 && tx.Statement.Schema != nil {
			var collect func(reflect.Value)
			collect = func(v reflect.Value) {
				for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
					if v.IsNil() {
						return
					}
					v = v.Elem()
				}
				if !v.IsValid() {
					return
				}
				if v.Kind() == reflect.Slice {
					for i := 0; i < v.Len(); i++ {
						collect(v.Index(i))
					}
					return
				}
				if v.Kind() == reflect.Struct {
					if field := tx.Statement.Schema.PrioritizedPrimaryField; field != nil {
						value, _ := field.ValueOf(tx.Statement.Context, v)
						switch id := value.(type) {
						case uint:
							if id != 0 {
								ids = append(ids, id)
							}
						case int64:
							if id != 0 {
								ids = append(ids, uint(id))
							}
						}
					}
				}
			}
			collect(tx.Statement.ReflectValue)
		}
		if err := syncUploadReferences(tx.Session(&gorm.Session{NewDB: true}), tx.Statement.Table, ids); err != nil {
			tx.AddError(err)
		}
	}
	createBefore := func(tx *gorm.DB) {
		if tx.Error == nil && uploadSources[tx.Statement.Table] != nil {
			tx.AddError(LockKey(tx.Session(&gorm.Session{NewDB: true}), uploadLock))
		}
	}
	if err := db.Callback().Create().Before("gorm:create").Register("uploads:lock", createBefore); err != nil {
		return err
	}
	if err := db.Callback().Create().After("gorm:create").Before("gorm:commit_or_rollback_transaction").Register("uploads:bind", after); err != nil {
		return err
	}
	if err := db.Callback().Update().Before("gorm:update").Register("uploads:lock", before); err != nil {
		return err
	}
	if err := db.Callback().Update().After("gorm:update").Before("gorm:commit_or_rollback_transaction").Register("uploads:bind", after); err != nil {
		return err
	}
	if err := db.Callback().Delete().Before("gorm:delete").Register("uploads:lock", before); err != nil {
		return err
	}
	return db.Callback().Delete().After("gorm:delete").Before("gorm:commit_or_rollback_transaction").Register("uploads:bind", after)
}

func syncUploadReferences(tx *gorm.DB, table string, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	columns := append([]string{"id"}, uploadSources[table]...)
	// Table and columns are fixed by uploadSources, never request strings.
	var rows []map[string]any
	query := tx.Table(table).Clauses(clause.Locking{Strength: "UPDATE"}).Select(columns).Where("id IN ?", ids)
	if table == "dishes" {
		// A dissolved family's recipes are no longer visible even in history.
		// Personal and still-accessible soft-deleted recipes retain their images.
		query = query.Where("family_id = 0 OR EXISTS (SELECT 1 FROM families WHERE families.id = dishes.family_id)")
	}
	if err := query.Find(&rows).Error; err != nil {
		return err
	}
	if err := tx.Where("source = ? AND source_id IN ?", table, ids).Delete(&models.UploadReference{}).Error; err != nil {
		return err
	}
	refs := []models.UploadReference{}
	for _, row := range rows {
		id := uint(0)
		switch v := row["id"].(type) {
		case uint:
			id = v
		case int64:
			id = uint(v)
		case uint64:
			id = uint(v)
		case int:
			id = uint(v)
		}
		if id == 0 {
			return errors.New("invalid upload source identity")
		}
		keys := map[string]bool{}
		for _, col := range uploadSources[table] {
			raw := ""
			switch v := row[col].(type) {
			case string:
				raw = v
			case []byte:
				raw = string(v)
			}
			var collect func(any)
			collect = func(v any) {
				switch v := v.(type) {
				case string:
					if key, ok := storage.KeyFromURL(v); ok {
						keys[key] = true
					}
				case []any:
					for _, item := range v {
						collect(item)
					}
				case map[string]any:
					for _, item := range v {
						collect(item)
					}
				}
			}
			var value any
			if strings.HasPrefix(raw, "[") || strings.HasPrefix(raw, "{") {
				if json.Unmarshal([]byte(raw), &value) == nil {
					collect(value)
				}
			} else {
				collect(raw)
			}
		}
		for key := range keys {
			var asset models.UploadAsset
			// A surrounding business transaction may already have an older RR
			// snapshot. Read the current tombstone after acquiring uploadLock.
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("`key` = ?", key).First(&asset).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil && (asset.State == "deleting" || asset.State == "deleted") {
				return ErrUploadGone
			}
			refs = append(refs, models.UploadReference{Source: table, SourceID: id, Key: key})
		}
	}
	if len(refs) > 0 {
		return tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&refs, 100).Error
	}
	return nil
}

func ReserveUpload(db *gorm.DB, uid uint, key, backup string, size int64) error {
	if size <= 0 || size > 10<<20 {
		return ErrUploadQuota
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := LockKey(tx, uploadLock); err != nil {
			return err
		}
		var user models.User
		if err := tx.First(&user, uid).Error; err != nil {
			return err
		}
		type totals struct {
			Bytes   int64
			Objects int64
		}
		for _, scope := range []struct {
			uid     uint
			bytes   int64
			objects int64
		}{{0, SiteUploadBytes, SiteUploadObjects}, {uid, UserUploadBytes, UserUploadObjects}} {
			var t totals
			q := tx.Model(&models.UploadAsset{}).Where("state <> ?", "deleted")
			if scope.uid != 0 {
				q = q.Where("owner_id = ?", scope.uid)
			}
			if err := q.Select("COALESCE(SUM(bytes),0) AS bytes, COUNT(*) AS objects").Scan(&t).Error; err != nil {
				return err
			}
			if t.Bytes > scope.bytes-size || t.Objects >= scope.objects {
				return ErrUploadQuota
			}
		}
		return tx.Create(&models.UploadAsset{OwnerID: uid, Key: key, BackupKey: backup, Bytes: size, State: "pending", ExpiresAt: time.Now().Add(24 * time.Hour)}).Error
	})
}

func FinishUpload(db *gorm.DB, key string) error {
	result := db.Model(&models.UploadAsset{}).Where("`key` = ? AND state = ?", key, "pending").Update("state", "ready")
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrUploadGone
	}
	return nil
}

// Claim before remote I/O. A tombstone prevents a new reference during delete;
// failed deletes stay charged and are retried by the next bounded cleanup run.
func DeleteUpload(ctx context.Context, db *gorm.DB, key string) error {
	var asset models.UploadAsset
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := LockKey(tx, uploadLock); err != nil {
			return err
		}
		var refs int64
		if err := tx.Model(&models.UploadReference{}).Where("`key` = ?", key).Count(&refs).Error; err != nil {
			return err
		}
		if refs > 0 {
			return ErrUploadReferenced
		}
		err := tx.Where("`key` = ?", key).First(&asset).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("图片尚未纳入存储账本，不能自动删除")
		}
		if err != nil {
			return err
		}
		if asset.State == "deleted" {
			return nil
		}
		return tx.Model(&asset).Update("state", "deleting").Error
	})
	if err != nil || asset.State == "deleted" {
		return err
	}
	if err := storage.Delete(ctx, key); err != nil {
		return err
	}
	if asset.BackupKey != "" {
		if err := storage.DeleteBackup(asset.BackupKey); err != nil {
			return err
		}
	}
	return db.Model(&models.UploadAsset{}).Where("id = ? AND state = ?", asset.ID, "deleting").Update("state", "deleted").Error
}

func CleanupUploads(ctx context.Context, db *gorm.DB) error {
	var assets []models.UploadAsset
	if err := db.Where("state = ? OR (state IN ? AND expires_at <= ?)", "deleting", []string{"pending", "ready"}, time.Now()).
		Where("NOT EXISTS (SELECT 1 FROM upload_references WHERE upload_references.`key` = upload_assets.`key`)").Order("id ASC").Limit(100).Find(&assets).Error; err != nil {
		return err
	}
	for _, a := range assets {
		if err := DeleteUpload(ctx, db, a.Key); err != nil && !errors.Is(err, ErrUploadReferenced) {
			return err
		}
	}
	return nil
}

// InitializeUploadLedger runs once during the single-writer schema migration.
// Historical object bytes are counted before the new quotas are enabled.
func InitializeUploadLedger(db *gorm.DB) error {
	if GetSetting("uploads_inventory_v1", "", db) == "done" {
		return nil
	}
	for table := range uploadSources {
		var after uint
		for {
			var rows []struct{ ID uint }
			if err := db.Table(table).Select("id").Where("id > ?", after).Order("id").Limit(100).Find(&rows).Error; err != nil {
				return err
			}
			if len(rows) == 0 {
				break
			}
			ids := make([]uint, len(rows))
			for i, r := range rows {
				ids[i] = r.ID
				after = r.ID
			}
			if err := db.Transaction(func(tx *gorm.DB) error {
				if err := LockKey(tx, uploadLock); err != nil {
					return err
				}
				return syncUploadReferences(tx, table, ids)
			}); err != nil {
				return err
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := storage.WalkUploads(ctx, func(key string, size int64) error {
		uid := storage.OwnerFromKey(key)
		if uid == 0 {
			return nil
		} // seeded/public files are not owned uploads
		asset := models.UploadAsset{Key: key, OwnerID: uid, Bytes: size, State: "ready", ExpiresAt: time.Now().Add(24 * time.Hour)}
		return db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"bytes"})}).Create(&asset).Error
	}); err != nil {
		return fmt.Errorf("upload inventory failed: %w", err)
	}
	if err := storage.WalkBackups(ctx, func(key string, size int64) error {
		uid := storage.OwnerFromKey(key)
		if uid == 0 {
			return nil
		}
		// Historical backup associations are ambiguous after format conversion.
		// Account for their bytes, retain them for an explicit backup-retention review.
		return db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&models.UploadAsset{Key: "__backup__/" + key, OwnerID: uid, Bytes: size, BackupKey: key, State: "archive"}).Error
	}); err != nil {
		return err
	}
	return SetSetting("uploads_inventory_v1", "done", db)
}
