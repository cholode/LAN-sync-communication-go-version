package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"lan-im-go/core"
	"lan-im-go/models"
	"lan-im-go/repository"
)

// AdminDeleteUser 管理员删除用户（下线+数据库软删除）
// 路由: DELETE /api/v1/admin/users/:id
func AdminDeleteUser(hub *core.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		targetUserIDStr := c.Param("id")
		targetUserID, err := strconv.ParseInt(targetUserIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "非法的用户 ID 参数"})
			return
		}

		// 1. 数据库软删除用户
		// 配合中间件，用户的JWT令牌后续请求将失效
		if err := repository.User.SoftDeleteUser(targetUserID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "用户删除失败，请查看日志"})
			return
		}

		// 2. 强制断开用户的WebSocket长连接
		// 若用户在线，立即关闭连接并禁止重连
		hub.Kick <- targetUserID

		c.JSON(http.StatusOK, gin.H{
			"msg": "用户已删除，在线连接已断开",
		})
	}
}

// AdminDeleteRoom 管理员解散群聊
// 路由: DELETE /api/v1/admin/rooms/:id
func AdminDeleteRoom(hub *core.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		targetRoomIDStr := c.Param("id")
		targetRoomID, err := strconv.ParseInt(targetRoomIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "非法的群聊 ID 参数"})
			return
		}

		// 1. 数据库软删除群聊
		// 删除后，该群聊的相关接口请求将被拦截
		if err := repository.Room.SoftDeleteRoom(targetRoomID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "解散群聊失败"})
			return
		}

		// 2. 向群内所有在线用户广播解散通知
		// 保证前端实时感知群聊状态变更，提升用户体验
		sysMsg := &models.Message{
			RoomID:   targetRoomID,
			SenderID: 0, // 系统标识
			Content:  "【系统通知】该群聊已被管理员解散",
		}
		// 通过核心引擎广播消息
		hub.Broadcast <- sysMsg

		c.JSON(http.StatusOK, gin.H{
			"msg": "群聊解散成功，已通知所有在线成员",
		})
	}
}
