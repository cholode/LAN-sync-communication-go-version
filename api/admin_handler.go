package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"lan-im-go/core"
	"lan-im-go/models"
	"lan-im-go/repository"
)

// AdminDeleteUser 物理级彻底封杀 (踢下线 + 软删数据库)
// 路由挂载: DELETE /api/v1/admin/users/:id
func AdminDeleteUser(hub *core.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		targetUserIDStr := c.Param("id")
		targetUserID, err := strconv.ParseInt(targetUserIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "非法的用户 ID 参数"})
			return
		}

		// 1. 斩断未来：在数据库中执行软删除 (Soft Delete)
		// 结合我们之前写的中间件，他手里的 JWT Token 将在下次 HTTP 请求时彻底作废
		if err := repository.User.SoftDeleteUser(targetUserID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库冻结用户失败，请查看日志"})
			return
		}

		// 2. 斩断现在：利用我们上一轮在 Hub 中加装的 Kick 通道
		// 如果他此刻正连在系统上，立刻在物理网络层强行拔掉他的 TCP 网线
		hub.Kick <- targetUserID

		c.JSON(http.StatusOK, gin.H{
			"msg": "账号已永久封禁，且全网物理切断了该用户的现有长连接",
		})
	}
}

// AdminDeleteRoom 超管一键解散/删除群聊
// 路由挂载: DELETE /api/v1/admin/rooms/:id
func AdminDeleteRoom(hub *core.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		targetRoomIDStr := c.Param("id")
		targetRoomID, err := strconv.ParseInt(targetRoomIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "非法的群聊 ID 参数"})
			return
		}

		// 1. 数据面阻断：数据库层面软删除群聊
		// 底层实现已经在之前写的 repository.Room.SoftDeleteRoom 中搞定
		// 从这一刻起，任何向该群聊发消息的 HTTP 校验都会因为找不到该群而拦截
		if err := repository.Room.SoftDeleteRoom(targetRoomID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "解散群聊失败"})
			return
		}

		// 2. 控制面同步 (架构师的绝杀细节)：向该群聊的所有在线成员广播“群聊解散”系统信令
		// 如果只删数据库，在线用户的群聊面板不会立刻消失，甚至还能在界面上打字，直到发送报错。
		// 工业级的做法是：主动下发一条带有特殊标记的系统消息。
		sysMsg := &models.Message{
			RoomID:   targetRoomID,
			SenderID: 0, // 0 通常代表系统最高级别的上帝视角消息
			Content:  "【系统通知】该群聊因违反相关规定，已被管理员强制解散",
			// 在真实的工业代码中，这里通常会加一个类似 MsgType: "system_cmd_room_dismiss"
		}
		// 投递给核心引擎，引擎会自动分发给目前还在该群里的所有存活连接
		hub.Broadcast <- sysMsg

		c.JSON(http.StatusOK, gin.H{
			"msg": "群聊解散成功，已向所有在线成员下发系统信令",
		})
	}
}
