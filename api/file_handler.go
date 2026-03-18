package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	//"strings"

	"github.com/gin-gonic/gin"
)

const (
	UploadBaseDir = "./data/uploads"
	TempChunkDir  = "./data/temp_chunks"
)

func InitFileDirs() {
	os.MkdirAll(UploadBaseDir, 0755)
	os.MkdirAll(TempChunkDir, 0755)
}

// ============================================================================
// 【断点续传：第 0 阶段 - 战损探针与秒传检测】
// 前端在上传任何文件前，必须先调用此接口
// ============================================================================
func CheckUploadStatus(c *gin.Context) {
	fileHash := c.Query("hash")
	fileName := c.Query("filename")
	if fileHash == "" || fileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数缺失"})
		return
	}

	safeFileName := filepath.Base(fileName)
	finalFilePath := filepath.Join(UploadBaseDir, fmt.Sprintf("%s_%s", fileHash, safeFileName))

	// 1. 终极白嫖 (秒传机制)：如果最终文件已经存在，直接告诉前端“传完了”，瞬间 100%
	if _, err := os.Stat(finalFilePath); err == nil {
		c.JSON(http.StatusOK, gin.H{
			"status":       "completed",
			"msg":          "文件已秒传",
			"download_url": fmt.Sprintf("/api/v1/download/%s_%s", fileHash, safeFileName),
		})
		return
	}

	// 2. 断点战损扫描：去临时目录清点尸体
	chunkDirPath := filepath.Join(TempChunkDir, filepath.Clean(fileHash))
	var uploadedChunks []int

	// 读取目录下的所有切片文件
	entries, err := os.ReadDir(chunkDirPath)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				// 将切片文件名 (如 "0", "1", "2") 解析为数字并存入数组
				if index, err := strconv.Atoi(entry.Name()); err == nil {
					uploadedChunks = append(uploadedChunks, index)
				}
			}
		}
	}

	// 3. 将已经存在的切片索引返回给前端，前端拿到后，只需上传缺少的切片
	c.JSON(http.StatusOK, gin.H{
		"status":          "uploading",
		"uploaded_chunks": uploadedChunks,
	})
}

// ============================================================================
// 【断点续传：第一阶段 - 接收分片 (支持并发写与覆盖写)】
// ============================================================================
func UploadChunk(c *gin.Context) {
	fileHash := c.PostForm("hash")
	chunkIndexStr := c.PostForm("chunk_index")
	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil || fileHash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "非法的分片参数"})
		return
	}

	fileHeader, err := c.FormFile("chunk")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法读取文件流"})
		return
	}

	chunkDirPath := filepath.Join(TempChunkDir, filepath.Clean(fileHash))
	os.MkdirAll(chunkDirPath, 0755)

	// OS 级魔法：如果之前有传了一半的废弃切片，这里会自动 O_TRUNC 截断并覆盖重写
	chunkFilePath := filepath.Join(chunkDirPath, strconv.Itoa(chunkIndex))
	if err := c.SaveUploadedFile(fileHeader, chunkFilePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "分片物理落盘失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"msg": fmt.Sprintf("分片 %d 上传成功", chunkIndex)})
}

// ============================================================================
// 【断点续传：第二阶段 - 物理级顺序合并】
// ============================================================================
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

	// 严苛防御：如果别人正在合并，或者已经合并完了，拒绝重复触发
	if _, err := os.Stat(finalFilePath); err == nil {
		c.JSON(http.StatusOK, gin.H{"msg": "文件已存在，无需重复合并"})
		return
	}

	finalFile, err := os.OpenFile(finalFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法创建最终文件"})
		return
	}
	defer finalFile.Close()

	chunkDirPath := filepath.Join(TempChunkDir, filepath.Clean(fileHash))

	// 必须严格串行写入，保证底层 SSD 发挥最大顺序写带宽，绝不并发！
	for i := 0; i < totalChunks; i++ {
		chunkFilePath := filepath.Join(chunkDirPath, strconv.Itoa(i))
		chunkFile, err := os.Open(chunkFilePath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("分片 %d 缺失，合并被强行中止", i)})
			// 发生严重断层，必须删掉这个半残的最终文件，防止脏数据累积
			os.Remove(finalFilePath)
			return
		}

		_, err = io.Copy(finalFile, chunkFile)
		chunkFile.Close()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "合并磁盘 I/O 异常"})
			os.Remove(finalFilePath)
			return
		}
	}

	// 阅后即焚：合并成功后抹除所有的临时切片垃圾
	os.RemoveAll(chunkDirPath)

	c.JSON(http.StatusOK, gin.H{
		"msg":          "文件合并成功",
		"download_url": fmt.Sprintf("/api/v1/download/%s_%s", fileHash, safeFileName),
	})
}

// ============================================================================
// 【极速下载：降维打击的 OS 级零拷贝】
// ============================================================================
func DownloadFile(c *gin.Context) {
	fileName := c.Param("filename")
	// 防御目录穿越攻击 (Path Traversal)
	// 如果黑客传 /download/../../../../etc/shadow，会被 Clean 掉并只取 Base
	safeFileName := filepath.Base(filepath.Clean(fileName))

	// 防止有人传空文件名试图拉取整个目录
	if safeFileName == "." || safeFileName == "/" || safeFileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "非法的文件请求"})
		return
	}

	filePath := filepath.Join(UploadBaseDir, safeFileName)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件已被超管物理销毁或不存在"})
		return
	}

	// 触发底层 sendfile 系统调用
	c.File(filePath)
}
