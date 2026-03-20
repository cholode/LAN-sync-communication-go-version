package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"lan-im-go/repository"
)

// GetChatHistory 获取群聊历史消息（游标分页）
// 路由：GET /api/v1/rooms/:id/messages?cursor=1050&limit=50
func GetChatHistory() gin.HandlerFunc {
	return func(c *gin.Context) {
		roomIDStr := c.Param("id")
		roomID, err := strconv.ParseInt(roomIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "非法的群聊 ID"})
			return
		}

		// 权限校验：仅群成员可查询消息
		userID := c.GetInt64("user_id")
		isMember, err := repository.RoomMember.CheckIsMember(roomID, userID)
		if err != nil || !isMember {
			c.JSON(http.StatusForbidden, gin.H{"error": "您不是该群成员，无权查看聊天记录"})
			return
		}

		// 解析分页参数
		// cursor=0 表示首次加载，查询最新消息
		cursorStr := c.DefaultQuery("cursor", "0")
		limitStr := c.DefaultQuery("limit", "50")
		cursorMsgID, _ := strconv.ParseInt(cursorStr, 10, 64)
		limit, _ := strconv.Atoi(limitStr)

		// 分页参数校验：限制最大查询数量，保证接口性能
		if limit > 100 {
			limit = 100
		} else if limit <= 0 {
			limit = 50
		}

		// 游标分页查询消息，避免深分页性能问题
		messages, err := repository.Message.GetHistoryByCursor(roomID, cursorMsgID, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "获取历史消息失败"})
			return
		}

		// 计算下一页游标
		var nextCursor int64 = 0
		if len(messages) > 0 {
			// 取当前列表最后一条消息ID作为下一页游标
			nextCursor = messages[len(messages)-1].ID
		}

		// 判断是否存在更多数据
		hasMore := len(messages) == limit

		c.JSON(http.StatusOK, gin.H{
			"messages":    messages,
			"next_cursor": nextCursor,
			"has_more":    hasMore,
		})
	}
}
