package api

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/text/unicode/norm"
	"log"
)

var (
	UploadBaseDir = filepath.Join(".", "data", "uploads")
	TempChunkDir  = filepath.Join(".", "data", "temp_chunks")
)

// isDirWritable 检测目录是否真实可写（已存在但无写权限时 MkdirAll 仍会“成功”）
func isDirWritable(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	f, err := os.CreateTemp(dir, ".lan-im-write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

func ensureWritableDir(primary string, fallbackParts ...string) string {
	_ = os.MkdirAll(primary, 0755)
	if isDirWritable(primary) {
		return primary
	}
	log.Printf("[目录] primary 不可写或创建失败，尝试回退: %s", primary)

	fallback := filepath.Join(append([]string{os.TempDir(), "lan-im-go"}, fallbackParts...)...)
	if err := os.MkdirAll(fallback, 0755); err == nil && isDirWritable(fallback) {
		log.Printf("[目录回退] 使用系统临时目录: %s", fallback)
		return fallback
	}

	if home, err := os.UserHomeDir(); err == nil {
		homeFallback := filepath.Join(append([]string{home, ".lan-im-go"}, fallbackParts...)...)
		if err := os.MkdirAll(homeFallback, 0755); err == nil && isDirWritable(homeFallback) {
			log.Printf("[目录回退] 使用用户主目录: %s", homeFallback)
			return homeFallback
		}
	}

	log.Printf("[目录错误] 所有候选路径均不可写，仍使用: %s", fallback)
	_ = os.MkdirAll(fallback, 0755)
	return fallback
}

func sanitizeHash(raw string) (string, error) {
	safeHash := filepath.Clean(raw)
	if safeHash == "." || safeHash == "/" || safeHash == "" || strings.Contains(safeHash, `\`) {
		return "", fmt.Errorf("参数非法")
	}
	return safeHash, nil
}

func getUserChunkDir(c *gin.Context) string {
	userID := c.GetInt64("user_id")
	return filepath.Join(TempChunkDir, strconv.FormatInt(userID, 10))
}

func chunkFileName(hash string, idx int) string {
	return fmt.Sprintf("%s_%d", hash, idx)
}

func parseChunkIndexByPrefix(fileName string, hashPrefix string) (int, bool) {
	if !strings.HasPrefix(fileName, hashPrefix) {
		return 0, false
	}
	part := strings.TrimPrefix(fileName, hashPrefix)
	idx, err := strconv.Atoi(part)
	if err != nil {
		return 0, false
	}
	return idx, true
}

func removeHashChunkFiles(chunkDirPath string, safeHash string) error {
	entries, err := os.ReadDir(chunkDirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	prefix := safeHash + "_"
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if err := os.Remove(filepath.Join(chunkDirPath, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// downloadSegmentFromRequest 从请求路径解析 /download/ 后的文件名（兼容通配路由与编码差异）。
func downloadSegmentFromRequest(c *gin.Context) string {
	path := c.Request.URL.Path
	marker := "/download/"
	idx := strings.LastIndex(path, marker)
	var enc string
	if idx >= 0 {
		enc = path[idx+len(marker):]
	}
	if enc == "" {
		enc = strings.TrimPrefix(strings.TrimSpace(c.Param("filepath")), "/")
	}
	if enc == "" {
		return ""
	}
	raw, err := url.PathUnescape(enc)
	if err != nil {
		raw = enc
	}
	return raw
}

func isHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// findUploadedObject 在 uploadDir 中按：精确保留名、小写、Unicode NFC、仅 SHA256 前缀唯一 等策略查找文件。
func findUploadedObject(uploadDir, logicalName string) (string, bool) {
	tryDirect := func(dir string) (string, bool) {
		p := filepath.Join(dir, logicalName)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, true
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return "", false
		}
		wantLow := strings.ToLower(logicalName)
		wantNFC := norm.NFC.String(logicalName)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			n := e.Name()
			if strings.ToLower(n) == wantLow || norm.NFC.String(n) == wantNFC {
				return filepath.Join(dir, n), true
			}
		}
		if len(logicalName) >= 65 && logicalName[64] == '_' {
			hash := logicalName[:64]
			if isHex64(hash) {
				prefix := hash + "_"
				var hits []string
				for _, e := range entries {
					if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
						hits = append(hits, e.Name())
					}
				}
				if len(hits) == 1 {
					return filepath.Join(dir, hits[0]), true
				}
			}
		}
		return "", false
	}

	if p, ok := tryDirect(uploadDir); ok {
		return p, true
	}

	legacy, err := filepath.Abs(filepath.Join(".", "data", "uploads"))
	if err != nil || strings.EqualFold(legacy, uploadDir) {
		return "", false
	}
	return tryDirect(legacy)
}

// clientDownloadName 磁盘文件名为「64位hex_原始名」时，向浏览器提供原始文件名以便另存为对话框显示合理名称。
func clientDownloadName(storedBase string) string {
	if len(storedBase) >= 65 && storedBase[64] == '_' {
		h := storedBase[:64]
		if isHex64(h) {
			return filepath.Base(storedBase[65:])
		}
	}
	return storedBase
}

// InitFileDirs 初始化文件上传目录。
// 可通过环境变量 LAN_IM_DATA_DIR 指定可写根目录（其下会创建 uploads、temp_chunks）。
func InitFileDirs() {
	if root := strings.TrimSpace(os.Getenv("LAN_IM_DATA_DIR")); root != "" {
		root = filepath.Clean(root)
		UploadBaseDir = filepath.Join(root, "uploads")
		TempChunkDir = filepath.Join(root, "temp_chunks")
		log.Printf("[目录] LAN_IM_DATA_DIR=%s -> uploads=%s temp_chunks=%s", root, UploadBaseDir, TempChunkDir)
	}
	UploadBaseDir = ensureWritableDir(UploadBaseDir, "uploads")
	TempChunkDir = ensureWritableDir(TempChunkDir, "temp_chunks")
	if abs, err := filepath.Abs(UploadBaseDir); err == nil {
		UploadBaseDir = abs
	}
	if abs, err := filepath.Abs(TempChunkDir); err == nil {
		TempChunkDir = abs
	}
	log.Printf("[目录] 最终 UploadBaseDir=%s TempChunkDir=%s", UploadBaseDir, TempChunkDir)
}

// func InitUserDir(UserID int) {
// 	os.MkdirAll(fmt.Sprintf("./data/temp_chunks/%s", strconv.Itoa(UserID)), 0755)
// }

// CheckUploadStatus 断点续传-文件状态校验（秒传检测）
// 前端上传文件前调用，校验文件是否已存在/已上传分片
func CheckUploadStatus(c *gin.Context) {
	fileHash := c.Query("hash")
	fileName := c.Query("filename")
	if fileHash == "" || fileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数缺失"})
		return
	}
	safeHash, err := sanitizeHash(fileHash)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数非法"})
		return
	}

	safeFileName := filepath.Base(fileName)
	finalFilePath := filepath.Join(UploadBaseDir, fmt.Sprintf("%s_%s", safeHash, safeFileName))

	// 1. 秒传检测：文件已存在，直接返回下载地址
	if _, err := os.Stat(finalFilePath); err == nil {
		c.JSON(http.StatusOK, gin.H{
			"status":       "completed",
			"msg":          "文件已存在，秒传成功",
			"download_url": fmt.Sprintf("/api/v1/download/%s_%s", safeHash, safeFileName),
		})
		return
	}

	// 2. 已上传分片扫描：获取临时目录中已上传的分片索引
	chunkDirPath := getUserChunkDir(c)
	var uploadedChunks []int

	entries, err := os.ReadDir(chunkDirPath)
	if err == nil {
		prefix := safeHash + "_"
		for _, entry := range entries {
			if !entry.IsDir() {
				if index, ok := parseChunkIndexByPrefix(entry.Name(), prefix); ok {
					uploadedChunks = append(uploadedChunks, index)
				}
			}
		}
	}

	// 3. 返回已上传分片，前端仅需上传缺失分片
	c.JSON(http.StatusOK, gin.H{
		"status":          "uploading",
		"uploaded_chunks": uploadedChunks,
	})
}

// UploadChunk 断点续传-分片上传
// 接收并存储单个文件分片，支持覆盖上传
func UploadChunk(c *gin.Context) {
	fileHash := c.PostForm("hash")
	chunkIndexStr := c.PostForm("chunk_index")
	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil || fileHash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "分片参数非法"})
		return
	}
	safeHash, err := sanitizeHash(fileHash)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "分片参数非法"})
		return
	}

	fileHeader, err := c.FormFile("chunk")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件流读取失败"})
		return
	}

	chunkDirPath := getUserChunkDir(c)
	if err := os.MkdirAll(chunkDirPath, 0755); err != nil {
		log.Printf("[上传错误] 创建分片目录失败 path=%s err=%v", chunkDirPath, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "分片目录创建失败: " + err.Error()})
		return
	}

	// 存储分片，自动覆盖已存在的分片文件
	chunkFilePath := filepath.Join(chunkDirPath, chunkFileName(safeHash, chunkIndex))
	src, err := fileHeader.Open()
	if err != nil {
		log.Printf("[上传错误] 打开上传流失败 idx=%d err=%v", chunkIndex, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "分片流打开失败: " + err.Error()})
		return
	}
	defer src.Close()

	dst, err := os.OpenFile(chunkFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		log.Printf("[上传错误] 创建分片文件失败 path=%s idx=%d err=%v", chunkFilePath, chunkIndex, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "分片存储失败: " + err.Error()})
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		log.Printf("[上传错误] 写入分片失败 path=%s idx=%d err=%v", chunkFilePath, chunkIndex, err)
		_ = os.Remove(chunkFilePath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "分片写入失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"msg": fmt.Sprintf("分片 %d 上传成功", chunkIndex)})
}

// MergeChunks 断点续传-分片合并
// 按顺序合并所有分片，生成完整文件，合并后清理临时分片
func MergeChunks(c *gin.Context) {
	fileHash := c.PostForm("hash")
	fileName := c.PostForm("filename")
	totalChunksStr := c.PostForm("total_chunks")
	totalChunks, err := strconv.Atoi(totalChunksStr)
	if err != nil || fileHash == "" || fileName == "" || totalChunks <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "合并参数缺失或非法"})
		return
	}
	safeHash, err := sanitizeHash(fileHash)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "合并参数缺失或非法"})
		return
	}

	safeFileName := filepath.Base(fileName)
	finalFilePath := filepath.Join(UploadBaseDir, fmt.Sprintf("%s_%s", safeHash, safeFileName))

	// 防重复合并：文件已存在则直接返回
	if _, err := os.Stat(finalFilePath); err == nil {
		c.JSON(http.StatusOK, gin.H{
			"msg":          "文件已存在，无需重复合并",
			"download_url": fmt.Sprintf("/api/v1/download/%s_%s", safeHash, safeFileName),
		})
		return
	}

	finalFile, err := os.OpenFile(finalFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "目标文件创建失败"})
		return
	}
	defer finalFile.Close()

	chunkDirPath := getUserChunkDir(c)

	// 按顺序合并分片，保证文件完整性
	for i := 0; i < totalChunks; i++ {
		chunkFilePath := filepath.Join(chunkDirPath, chunkFileName(safeHash, i))
		chunkFile, err := os.Open(chunkFilePath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("分片 %d 缺失，合并终止", i)})
			os.Remove(finalFilePath)
			return
		}

		_, err = io.Copy(finalFile, chunkFile)
		chunkFile.Close()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "文件合并IO异常"})
			os.Remove(finalFilePath)
			return
		}
	}

	// 合并完成，清理当前文件对应的临时分片
	if err := removeHashChunkFiles(chunkDirPath, safeHash); err != nil {
		log.Printf("[上传清理错误] 清理分片失败 path=%s hash=%s err=%v", chunkDirPath, safeHash, err)
	}

	c.JSON(http.StatusOK, gin.H{
		"msg":          "文件合并成功",
		"download_url": fmt.Sprintf("/api/v1/download/%s_%s", safeHash, safeFileName),
	})
}

// DownloadFile 文件下载接口（注册在公开路由组，无需 JWT）。
// 对象名为「整文件 SHA-256 + 原始文件名」，相当于难猜测的能力链接，便于群成员在浏览器中直接下载。
// 安全校验文件路径，防止目录遍历攻击。
func DownloadFile(c *gin.Context) {
	raw := downloadSegmentFromRequest(c)
	safeFileName := filepath.Base(filepath.Clean(raw))

	if safeFileName == "." || safeFileName == "/" || safeFileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件请求非法"})
		return
	}

	if filePath, ok := findUploadedObject(UploadBaseDir, safeFileName); ok {
		c.FileAttachment(filePath, clientDownloadName(safeFileName))
		return
	}

	log.Printf("[download] 未找到文件 want=%s uploadDir=%s path=%s param=%q",
		safeFileName, UploadBaseDir, c.Request.URL.Path, c.Param("filepath"))
	c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
}

// CancelUpload 取消文件上传
// 终止上传流程，清理对应的临时分片文件
func CancelUpload(c *gin.Context) {
	fileHash := c.Query("hash")
	if fileHash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数缺失"})
		return
	}

	// 安全防护：校验文件哈希合法性
	safeHash, err := sanitizeHash(fileHash)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数非法"})
		return
	}

	chunkDirPath := getUserChunkDir(c)

	// 清理临时分片文件
	if err := removeHashChunkFiles(chunkDirPath, safeHash); err != nil {
		log.Printf("[上传清理错误] 临时目录删除失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "临时文件清理失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"msg": "上传已终止，临时文件已清理"})
}
