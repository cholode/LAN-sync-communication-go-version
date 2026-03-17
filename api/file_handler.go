package api

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"lan-im-go/core" // 引入你的 Hub 所在的包
)

// UploadFile 处理大文件上传，并在落盘后触发全群广播
// 严苛细节：通过闭包将全局的 hub 实例注入到 Gin 的 Handler 中
func UploadFile(hub *core.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 解析业务参数 (发送者ID, 目标房间ID)
		senderIDStr := c.PostForm("sender_id")
		roomIDStr := c.PostForm("room_id")
		senderID, _ := strconv.ParseInt(senderIDStr, 10, 64)
		roomID, _ := strconv.ParseInt(roomIDStr, 10, 64)
		if senderID == 0 || roomID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺失关键业务参数"})
			return
		}

		// 2. 接收文件流
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "获取文件失败"})
			return
		}

		// 严苛细节：为了防止同名文件覆盖，必须重命名文件 (这里用时间戳简易代替)
		filename := fmt.Sprintf("%d_%s", time.Now().UnixNano(), filepath.Base(file.Filename))
		savePath := filepath.Join("./data/uploads", filename)

		// 3. 执行物理落盘 (这里 Gin 底层会使用 io.Copy 高效写入)
		if err := c.SaveUploadedFile(file, savePath); err != nil {
			log.Printf("[Upload] 文件落盘失败: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "文件保存失败"})
			return
		}

		// 4. 落盘成功！开始构造要广播的群聊消息
		// 假设你的 Nginx 或后端下载接口配置为 /api/download/xxx
		downloadURL := fmt.Sprintf("/api/download/%s", filename)
		msg := &core.Message{
			MsgID:    time.Now().UnixNano(), // 实际应替换为雪花算法 ID
			SenderID: senderID,
			RoomID:   roomID,
			// 组装供前端解析的特制文件消息结构
			Content: fmt.Sprintf(`{"type":"file", "name":"%s", "url":"%s", "size":%d}`, file.Filename, downloadURL, file.Size),
		}

		// 5. 跨模块异步投递：将消息扔给 Hub 进行内存路由和异步入库
		// 注意这里是非阻塞投递，保证 HTTP 接口能极速向前端返回 200 OK
		select {
		case hub.Broadcast <- msg:
			log.Printf("[Upload] 文件 %s 落盘完毕，已通知 Hub 广播至 Room %d", filename, roomID)
		default:
			log.Printf("[Upload 警告] Hub Broadcast 队列满，文件通知投递失败！")
		}

		// 6. 响应前端 HTTP 请求
		c.JSON(http.StatusOK, gin.H{
			"msg": "文件上传成功并已广播",
			"url": downloadURL,
		})
	}
}

// Downloadfile 就是你提到的下载方法，利用 Go 底层极速的零拷贝/流式下发
func Downloadfile() gin.HandlerFunc {
	return func(c *gin.Context) {
		filename := c.Param("filename")
		// 防止目录穿越漏洞 (Directory Traversal) 的安全校验
		cleanPath := filepath.Clean(filepath.Join("./data/uploads", filename))
		// Gin 内置的极强方法，自动处理 MIME 类型和分块传输
		c.File(cleanPath)
	}
}
