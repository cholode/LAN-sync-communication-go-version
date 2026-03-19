package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"lan-im-go/core"
	"lan-im-go/repository"
)

// JoinRoom 处理加入群聊的 HTTP 请求
func JoinRoom(hub *core.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 提取路由参数
		roomID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "非法的群聊 ID"})
			return
		}

		// 2. 提取当前操作者身份 (严禁从 Body 里取 UserID)
		userIDVal, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "身份防弹衣丢失"})
			return
		}
		realUserID := userIDVal.(int64)

		// 3. 执行幂等落盘 (默认给 Role: 0 普通成员)
		// 依赖你刚修好的 OnConflict DoNothing 或者 Duplicate Entry 拦截
		if err := repository.RoomMember.AddMember(roomID, realUserID, 0); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "系统异常，物理落盘失败"})
			return
		}

		// 4. ⚡ 架构师杀招：跨协程内存状态同步
		// 向 Hub 引擎发射控制信令，不掉线热更新 WebSocket 路由表
		hub.RoomActionChan <- &core.RoomAction{
			UserID: realUserID,
			RoomID: roomID,
			Action: "join",
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "成功接入该群聊频段",
			"room_id": roomID,
		})
	}
}

// RemoveRoomMember 统一退群/踢人接口 (极其严苛的仲裁庭)
func RemoveRoomMember(hub *core.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		roomID, err1 := strconv.ParseInt(c.Param("id"), 10, 64)
		targetUserID, err2 := strconv.ParseInt(c.Param("user_id"), 10, 64) // 被踢的人
		if err1 != nil || err2 != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "非法的参数格式"})
			return
		}

		// 提取当前操作者身份与全局权限
		operatorID := c.GetInt64("user_id")
		globalRole := c.GetInt8("user_role") // 1:全局超管, 0:普通用户

		// ==========================================
		// 🛡️ RBAC 越权防御矩阵
		// ==========================================
		if globalRole != 1 {
			// 如果不是超管，进入凡人仲裁逻辑
			if operatorID != targetUserID {
				// 试图踢别人
				// 这里需要极其严苛的校验：你必须是该群的群主 (Role = 2) 才能踢人
				// 注意：这里需要你的 repo 提供一个获取单人 Role 的方法，为了简化演示，
				// 我们假设普通人绝对不允许踢别人。如果你后续完善了 GetMemberInfo，请在这里拦截。
				c.JSON(http.StatusForbidden, gin.H{"error": "越权警告：你无权驱逐其他成员！"})
				return
			}
		}

		// ==========================================
		// 💥 执行剥离 (由于你提供的 repo 只有 RemoveMember，我们先执行基础驱逐)
		// ==========================================
		err := repository.RoomMember.RemoveMember(roomID, targetUserID)
		if err != nil {
			// 拦截上一轮我们讨论的 RowsAffected == 0 的情况
			if err.Error() == "record not found" || err.Error() == "该成员不在群聊中" {
				c.JSON(http.StatusNotFound, gin.H{"error": "该用户本就不在群内，剥离无效"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "人员驱逐失败"})
			return
		}

		// ⚡ 跨协程内存剥离：让 Hub 瞬间掐断该用户在该群的实时监听
		hub.RoomActionChan <- &core.RoomAction{
			UserID: targetUserID,
			RoomID: roomID,
			Action: "leave",
		}

		c.JSON(http.StatusOK, gin.H{"message": "物理隔离执行完毕"})
	}
}

// GetRoomMembers 获取群成员列表 (供前端渲染侧边栏)
func GetRoomMembers() gin.HandlerFunc {
	return func(c *gin.Context) {
		roomID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "非法的群聊 ID"})
			return
		}

		// ⚡ 越权防线：只有本群的人，才能偷窥本群的名单
		userIDVal, _ := c.Get("user_id")
		isMember, err := repository.RoomMember.CheckIsMember(roomID, userIDVal.(int64))
		if err != nil || !isMember {
			c.JSON(http.StatusForbidden, gin.H{"error": "你未加入该群，无权获取花名册"})
			return
		}

		// 极速连表查询
		users, err := repository.RoomMember.GetRoomMembers(roomID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "拉取数据失败"})
			return
		}

		// 组装脱敏后的数据返回给前端
		var response []map[string]interface{}
		for _, u := range users {
			response = append(response, map[string]interface{}{
				"user_id":  u.ID,
				"username": u.Username,
				// "avatar": u.Avatar, // 如果有头像字段
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"room_id": roomID,
			"members": response,
		})
	}
}
