package api

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"lan-im-go/core"
	"lan-im-go/models"
	"lan-im-go/repository"
	"log"
	"net/http"
	"strconv"
)

// JoinRoom 加入群聊
// 路由：POST /api/v1/rooms/:id/join
func JoinRoom(hub *core.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		roomID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "非法的群聊 ID"})
			return
		}

		userIDVal, _ := c.Get("user_id")
		realUserID := userIDVal.(int64)

		// 校验群聊是否存在
		_, err = repository.Room.GetRoomByID(roomID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "群聊不存在或已被删除"})
			return
		}

		// 添加群成员（幂等操作）
		if err := repository.RoomMember.AddMember(roomID, realUserID, 0); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "加入群聊失败"})
			return
		}

		// 通知核心引擎更新群聊状态
		hub.RoomActionChan <- &core.RoomAction{
			UserID: realUserID,
			RoomID: roomID,
			Action: "join",
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "加入群聊成功",
			"room_id": roomID,
		})
	}
}

// RemoveRoomMember 移除群成员/退出群聊
// 路由：DELETE /api/v1/rooms/:id/members/:user_id
func RemoveRoomMember(hub *core.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		roomID, err1 := strconv.ParseInt(c.Param("id"), 10, 64)
		targetUserID, err2 := strconv.ParseInt(c.Param("user_id"), 10, 64)
		if err1 != nil || err2 != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "参数格式非法"})
			return
		}

		// 获取当前操作者信息
		operatorID := c.GetInt64("user_id")
		globalRole := c.GetInt8("user_role") // 1=管理员,0=普通用户

		// 权限校验：非管理员只能操作自己的账号
		if globalRole != 1 {
			if operatorID != targetUserID {
				c.JSON(http.StatusForbidden, gin.H{"error": "权限不足，无法移除其他成员"})
				return
			}
		}

		// 执行移除成员操作
		err := repository.RoomMember.RemoveMember(roomID, targetUserID)
		if err != nil {
			if err.Error() == "record not found" || err.Error() == "该成员不在群聊中" {
				c.JSON(http.StatusNotFound, gin.H{"error": "用户不在群聊中"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "移除成员失败"})
			return
		}

		// 通知核心引擎更新群聊状态
		hub.RoomActionChan <- &core.RoomAction{
			UserID: targetUserID,
			RoomID: roomID,
			Action: "leave",
		}

		c.JSON(http.StatusOK, gin.H{"message": "操作成功"})
	}
}

// GetRoomMembers 获取群成员列表
// 路由：GET /api/v1/rooms/:id/members
func GetRoomMembers() gin.HandlerFunc {
	return func(c *gin.Context) {
		roomID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "非法的群聊 ID"})
			return
		}

		// 权限校验：仅群成员可查看成员列表
		userIDVal, _ := c.Get("user_id")
		isMember, err := repository.RoomMember.CheckIsMember(roomID, userIDVal.(int64))
		if err != nil || !isMember {
			c.JSON(http.StatusForbidden, gin.H{"error": "未加入该群，无权查看成员列表"})
			return
		}

		// 查询群成员信息
		users, err := repository.RoomMember.GetRoomMembers(roomID)
		// #region agent log
		errPl := ""
		if err != nil {
			errPl = err.Error()
		}
		agentDebugLog("H4-verify", "room_handler.go:GetRoomMembers:afterRepo", "member query", map[string]any{"roomID": roomID, "count": len(users), "err": errPl})
		// #endregion
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "获取成员列表失败"})
			return
		}

		// 组装脱敏响应数据
		var response []map[string]interface{}
		for _, u := range users {
			response = append(response, map[string]interface{}{
				"user_id":  u.ID,
				"username": u.Username,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"room_id": roomID,
			"members": response,
		})
	}
}

// CreateRoom 创建群聊
// 路由：POST /api/v1/rooms
func CreateRoom(hub *core.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Name string `json:"name" binding:"required"`
		}
		// 参数校验
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "群聊名称不能为空"})
			return
		}

		// 从令牌中获取创建者ID，保证身份安全
		userIDVal, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "身份验证失败"})
			return
		}
		creatorID := userIDVal.(int64)

		// 构建群聊数据
		room := &models.Room{
			Name:      req.Name,
			CreatorID: creatorID,
			Type:      2, // 2=普通群聊
		}

		// 事务创建群聊并添加创建者为成员
		if err := repository.Room.CreateRoomWithCreator(room, creatorID); err != nil {
			log.Printf("创建群聊事务执行失败: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建群聊失败"})
			return
		}

		// 通知核心引擎更新群聊状态
		hub.RoomActionChan <- &core.RoomAction{
			UserID: creatorID,
			RoomID: room.ID,
			Action: "join",
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "创建群聊成功",
			"room_id": room.ID,
			"name":    room.Name,
		})
	}
}

// GetMyRooms 获取当前用户加入的所有群聊
// 路由：GET /api/v1/my_rooms
func GetMyRooms() gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDVal, uidOk := c.Get("user_id")
		// #region agent log
		typeStr := ""
		if userIDVal != nil {
			typeStr = fmt.Sprintf("%T", userIDVal)
		}
		agentDebugLog("H6", "room_handler.go:GetMyRooms:entry", "user_id from context", map[string]any{"hasUserID": uidOk, "goType": typeStr})
		// #endregion
		userID := userIDVal.(int64)

		// 查询用户加入的群聊列表
		rooms, err := repository.Room.GetJoinedRooms(userID)
		// #region agent log
		errStr := ""
		if err != nil {
			errStr = err.Error()
		}
		agentDebugLog("H3", "room_handler.go:GetMyRooms:afterQuery", "GetJoinedRooms", map[string]any{"userID": userID, "err": errStr, "count": len(rooms)})
		// #endregion
		if err != nil {
			log.Printf("查询群聊列表失败: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "获取群聊列表失败"})
			return
		}

		// 组装响应数据
		var res []map[string]interface{}
		for _, r := range rooms {
			res = append(res, map[string]interface{}{
				"room_id":    r.ID,
				"room_name":  r.Name,
				"created_at": r.CreatedAt,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"rooms": res,
		})
	}
}
