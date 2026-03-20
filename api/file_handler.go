package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
	"log"
)

const (
	UploadBaseDir = "./data/uploads"
	TempChunkDir  = "./data/temp_chunks"
)

// InitFileDirs 初始化文件上传目录
func InitFileDirs() {
	os.MkdirAll(UploadBaseDir, 0755)
	os.MkdirAll(TempChunkDir, 0755)
}

// CheckUploadStatus 断点续传-文件状态校验（秒传检测）
// 前端上传文件前调用，校验文件是否已存在/已上传分片
func CheckUploadStatus(c *gin.Context) {
	fileHash := c.Query("hash")
	fileName := c.Query("filename")
	if fileHash == "" || fileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数缺失"})
		return
	}

	safeFileName := filepath.Base(fileName)
	finalFilePath := filepath.Join(UploadBaseDir, fmt.Sprintf("%s_%s", fileHash, safeFileName))

	// 1. 秒传检测：文件已存在，直接返回下载地址
	if _, err := os.Stat(finalFilePath); err == nil {
		c.JSON(http.StatusOK, gin.H{
			"status":       "completed",
			"msg":          "文件已存在，秒传成功",
			"download_url": fmt.Sprintf("/api/v1/download/%s_%s", fileHash, safeFileName),
		})
		return
	}

	// 2. 已上传分片扫描：获取临时目录中已上传的分片索引
	chunkDirPath := filepath.Join(TempChunkDir, filepath.Clean(fileHash))
	var uploadedChunks []int

	entries, err := os.ReadDir(chunkDirPath)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				if index, err := strconv.Atoi(entry.Name()); err == nil {
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

	fileHeader, err := c.FormFile("chunk")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件流读取失败"})
		return
	}

	chunkDirPath := filepath.Join(TempChunkDir, filepath.Clean(fileHash))
	os.MkdirAll(chunkDirPath, 0755)

	// 存储分片，自动覆盖已存在的分片文件
	chunkFilePath := filepath.Join(chunkDirPath, strconv.Itoa(chunkIndex))
	if err := c.SaveUploadedFile(fileHeader, chunkFilePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "分片存储失败"})
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

	safeFileName := filepath.Base(fileName)
	finalFilePath := filepath.Join(UploadBaseDir, fmt.Sprintf("%s_%s", fileHash, safeFileName))

	// 防重复合并：文件已存在则直接返回
	if _, err := os.Stat(finalFilePath); err == nil {
		c.JSON(http.StatusOK, gin.H{"msg": "文件已存在，无需重复合并"})
		return
	}

	finalFile, err := os.OpenFile(finalFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "目标文件创建失败"})
		return
	}
	defer finalFile.Close()

	chunkDirPath := filepath.Join(TempChunkDir, filepath.Clean(fileHash))

	// 按顺序合并分片，保证文件完整性
	for i := 0; i < totalChunks; i++ {
		chunkFilePath := filepath.Join(chunkDirPath, strconv.Itoa(i))
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

	// 合并完成，清理临时分片目录
	os.RemoveAll(chunkDirPath)

	c.JSON(http.StatusOK, gin.H{
		"msg":          "文件合并成功",
		"download_url": fmt.Sprintf("/api/v1/download/%s_%s", fileHash, safeFileName),
	})
}

// DownloadFile 文件下载接口
// 安全校验文件路径，防止目录遍历攻击，使用系统零拷贝提升下载性能
func DownloadFile(c *gin.Context) {
	fileName := c.Param("filename")
	// 安全防护：防止目录遍历攻击
	safeFileName := filepath.Base(filepath.Clean(fileName))

	if safeFileName == "." || safeFileName == "/" || safeFileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件请求非法"})
		return
	}

	filePath := filepath.Join(UploadBaseDir, safeFileName)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}

	// 调用系统sendfile零拷贝传输文件
	c.File(filePath)
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
	safeHash := filepath.Clean(fileHash)
	if safeHash == "." || safeHash == "/" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数非法"})
		return
	}

	chunkDirPath := filepath.Join(TempChunkDir, safeHash)

	// 清理临时分片文件
	if err := os.RemoveAll(chunkDirPath); err != nil {
		log.Printf("[上传清理错误] 临时目录删除失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "临时文件清理失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"msg": "上传已终止，临时文件已清理"})
}
