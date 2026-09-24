package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"ninimenu/internal/config"
	imgproc "ninimenu/internal/imaging"
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

// UploadImage 上传图片（任何登录用户）：内容嗅探校验 → 压缩为 JPG → 写入对象存储（或本地）。
// 本地模式下原图另存到 BACKUP_DIR 以备恢复；对象存储模式只保存压缩结果。
func UploadImage(c *gin.Context) {
	file, err := c.FormFile("image")
	if err != nil {
		utils.BadRequest(c, "请选择图片")
		return
	}
	maxMB := config.C.MaxUploadSize / 1024 / 1024
	if file.Size > config.C.MaxUploadSize {
		utils.BadRequest(c, fmt.Sprintf("图片大小不能超过%dMB", maxMB))
		return
	}

	src, err := file.Open()
	if err != nil {
		utils.BadRequest(c, "读取图片失败")
		return
	}
	data, err := io.ReadAll(io.LimitReader(src, config.C.MaxUploadSize+1))
	src.Close()
	if err != nil || int64(len(data)) > config.C.MaxUploadSize {
		utils.BadRequest(c, fmt.Sprintf("图片大小不能超过%dMB", maxMB))
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
	if compressed, err := imgproc.CompressJPEG(data, config.C.CompressMaxDim, config.C.JpegQuality); err == nil {
		body, contentType, name = compressed, "image/jpeg", baseName+".jpg"
	} else {
		log.Printf("[upload] 压缩失败，保存原图: %v", err)
	}

	if !storage.UsingS3() && config.C.BackupDir != "" {
		backup := filepath.Join(config.C.BackupDir, fmt.Sprintf("u/%d", uid(c)), now.Format("2006/01/02"), baseName+ext)
		if err := os.MkdirAll(filepath.Dir(backup), 0755); err == nil {
			_ = os.WriteFile(backup, data, 0644)
		}
	}

	key := storage.UserKey(uid(c), name, now)
	url, err := storage.Save(c.Request.Context(), key, body, contentType)
	if err != nil {
		log.Printf("[upload] 保存 %s 失败: %v", key, err)
		utils.InternalError(c, "图片保存失败，请稍后再试")
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
	if err := storage.Delete(c.Request.Context(), key); err != nil {
		log.Printf("[upload] 删除 %s 失败: %v", key, err)
		utils.InternalError(c, "删除失败")
		return
	}
	utils.SuccessMsg(c, "删除成功")
}
