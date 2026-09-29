package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"ninimenu/internal/config"
	"ninimenu/internal/database"
	imgproc "ninimenu/internal/imaging"
	"ninimenu/internal/resourcebudget"
	"ninimenu/internal/storage"
	"ninimenu/internal/utils"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func randomSuffix() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

var allowedImageTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

var (
	imageUploadSlot    = make(chan struct{}, 1)
	imageUploadWaiters = make(chan struct{}, 4)
	errUploadBusy      = errors.New("image upload busy")
	errUploadTooLarge  = errors.New("image upload too large")
	errInvalidUpload   = errors.New("expected one image file")
)

// heavyUploadWait lets a decode queue behind another decode or the tail of an
// ASR job, within the upload's 30 second request budget, instead of failing at once.
const heavyUploadWait = 20 * time.Second

// Admit before reading multipart data, so queued uploads do not retain image
// buffers. Both the queue and wait are bounded; clients can retry a busy upload.
func acquireImageUpload(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case imageUploadSlot <- struct{}{}:
		return nil
	default:
	}
	select {
	case imageUploadWaiters <- struct{}{}:
		defer func() { <-imageUploadWaiters }()
	default:
		return errUploadBusy
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case imageUploadSlot <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return errUploadBusy
	}
}

// Stream parts into a single bounded file buffer. FormFile/ParseMultipartForm
// would retain an additional in-memory copy (or create a temporary disk file).
func readUploadImage(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, error) {
	const overhead = 64 << 10
	if limit <= 0 || r.ContentLength > limit+overhead {
		return nil, errUploadTooLarge
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit+overhead)
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, errInvalidUpload
	}
	var data []byte
	seenImage := false
	for parts := 0; ; parts++ {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if parts >= 8 {
			return nil, errInvalidUpload
		}
		if part.FileName() != "" {
			if part.FormName() != "image" || seenImage {
				return nil, errInvalidUpload
			}
			seenImage = true
			var buf bytes.Buffer
			if r.ContentLength > 0 {
				// ReadFrom requests MinRead bytes even when checking for EOF.
				buf.Grow(int(min(r.ContentLength, limit+1)) + bytes.MinRead)
			}
			if _, err := buf.ReadFrom(io.LimitReader(part, limit+1)); err != nil {
				return nil, err
			}
			if int64(buf.Len()) > limit {
				return nil, errUploadTooLarge
			}
			data = buf.Bytes()
		} else if _, err := io.Copy(io.Discard, part); err != nil {
			return nil, err
		}
		if err := part.Close(); err != nil {
			return nil, err
		}
	}
	if len(data) == 0 {
		return nil, errInvalidUpload
	}
	return data, nil
}

// UploadImage 上传图片（任何登录用户）：内容嗅探校验 → 压缩为 JPG → 写入对象存储（或本地）。
// 本地模式下原图另存到 BACKUP_DIR 以备恢复；对象存储模式只保存压缩结果。
func UploadImage(c *gin.Context) {
	if err := acquireImageUpload(c.Request.Context()); err != nil {
		if c.Request.Context().Err() == nil {
			c.Header("Retry-After", "5")
			utils.Error(c, http.StatusTooManyRequests, 42900, "图片上传正忙，请稍后重试")
		}
		return
	}
	defer func() { <-imageUploadSlot }()
	data, err := readUploadImage(c.Writer, c.Request, config.C.MaxUploadSize)
	if err != nil {
		var bodyLimit *http.MaxBytesError
		if errors.Is(err, errUploadTooLarge) || errors.As(err, &bodyLimit) {
			utils.Error(c, http.StatusRequestEntityTooLarge, 41300, fmt.Sprintf("图片大小不能超过%dMB，请缩小图片后重试", config.C.MaxUploadSize/1024/1024))
		} else {
			utils.BadRequest(c, "读取图片失败，请重新选择一张图片")
		}
		return
	}

	// 以文件内容为准判断类型，不信任扩展名与客户端声明的 Content-Type
	sniffed := http.DetectContentType(data)
	ext, ok := allowedImageTypes[sniffed]
	if !ok {
		utils.BadRequest(c, "仅支持 jpg/png/webp 格式")
		return
	}

	now := time.Now()
	suffix, err := randomSuffix()
	if err != nil {
		utils.InternalError(c, "图片保存失败，请稍后再试")
		return
	}
	baseName := fmt.Sprintf("%d-%s", now.UnixMilli(), suffix)
	body, contentType, name := data, sniffed, baseName+ext
	// Only decoding needs the shared memory reservation; reading the body and the
	// storage write do not, so a slow client or bucket cannot block ASR or others.
	release, err := resourcebudget.AcquireWait(c.Request.Context(), heavyUploadWait)
	if err != nil {
		if c.Request.Context().Err() == nil {
			c.Header("Retry-After", "5")
			utils.Error(c, 429, 42900, "图片处理繁忙，请稍后重试")
		}
		return
	}
	compressed, err := imgproc.CompressJPEG(data, config.C.CompressMaxDim, config.C.JpegQuality)
	release()
	if err == nil {
		body, contentType, name = compressed, "image/jpeg", baseName+".jpg"
	} else if errors.Is(err, imgproc.ErrImageTooLarge) {
		utils.BadRequest(c, "图片分辨率过高，请缩小到2400万像素以内、单边不超过16384像素后重试")
		return
	} else {
		log.Printf("[upload] 压缩失败，保存原图: %v", err)
	}

	key := storage.UserKey(uid(c), name, now)
	backupKey := ""
	size := int64(len(body))
	if !storage.UsingS3() && config.C.BackupDir != "" {
		backupKey = fmt.Sprintf("u/%d/%s/%s", uid(c), now.Format("2006/01/02"), baseName+ext)
		size += int64(len(data))
	}
	db := database.DB.WithContext(c.Request.Context())
	if err := database.ReserveUpload(db, uid(c), key, backupKey, size); err != nil {
		if errors.Is(err, database.ErrUploadQuota) {
			utils.Error(c, 429, 42900, err.Error())
		} else {
			utils.InternalError(c, "图片存储暂不可用")
		}
		return
	}
	if backupKey != "" {
		backup := filepath.Join(config.C.BackupDir, filepath.FromSlash(backupKey))
		if err := os.MkdirAll(filepath.Dir(backup), 0755); err != nil {
			utils.InternalError(c, "图片保存失败")
			return
		}
		if err := os.WriteFile(backup, data, 0644); err != nil {
			utils.InternalError(c, "图片保存失败")
			return
		}
	}
	url, err := storage.Save(c.Request.Context(), key, body, contentType)
	if err != nil {
		log.Printf("[upload] 保存 %s 失败: %v", key, err)
		utils.InternalError(c, "图片保存失败，请稍后再试")
		return
	}
	if err := database.FinishUpload(db, key); err != nil {
		utils.InternalError(c, "图片保存失败，请重试")
		return
	}
	utils.Success(c, gin.H{"url": url, "filename": name, "size": len(body), "storage": storage.Backend()})
}

// DeleteImage 删除图片：普通用户只能删自己目录下的；种子图片与他人图片只有管理员能删。
func DeleteImage(c *gin.Context) {
	var req struct {
		URL string `json:"url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请提供图片URL")
		return
	}
	key, ok := storage.KeyFromURL(req.URL)
	if !ok || strings.Contains(key, "..") {
		utils.BadRequest(c, "图片URL无效")
		return
	}
	if !isAdmin(c) && !storage.OwnedBy(key, uid(c)) {
		utils.Forbidden(c, "只能删除自己上传的图片")
		return
	}
	if err := database.DeleteUpload(c.Request.Context(), database.DB.WithContext(c.Request.Context()), key); err != nil {
		if errors.Is(err, database.ErrUploadReferenced) {
			utils.Error(c, 409, 40900, err.Error())
		} else {
			log.Printf("[upload] 删除 %s 失败: %v", key, err)
			utils.InternalError(c, "删除失败")
		}
		return
	}
	utils.SuccessMsg(c, "删除成功")
}
