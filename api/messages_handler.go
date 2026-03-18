package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"lan-im-go/repository"
)

// GetChatHistory 拉取房间历史消息 (游标分页)
// 路由挂载: GET /api/v1/rooms/:id/messages?cursor=1050&limit=50
func GetChatHistory() gin.HandlerFunc {
	return func(c *gin.Context) {
		roomIDStr := c.Param("id")
		roomID, err := strconv.ParseInt(roomIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "非法的群聊 ID"})
			return
		}

		// 1. 越权防线：绝不允许非群成员拉取聊天记录 (即使他有合法的 JWT)
		userID := c.GetInt64("user_id")
		isMember, err := repository.RoomMember.CheckIsMember(roomID, userID)
		if err != nil || !isMember {
			c.JSON(http.StatusForbidden, gin.H{"error": "您不是该群成员，无权查看聊天记录"})
			return
		}

		// 2. 解析游标与分页参数
		// cursor: 当前屏幕最老的那条消息的 ID。传 0 代表第一次进群，拉取最新消息
		cursorStr := c.DefaultQuery("cursor", "0")
		limitStr := c.DefaultQuery("limit", "50") // 严苛控制：默认一次最多拉 50 条，防止撑爆内存
		cursorMsgID, _ := strconv.ParseInt(cursorStr, 10, 64)
		limit, _ := strconv.Atoi(limitStr)

		// 边界防御：即使前端传了 limit=10000，后端也必须强行截断
		if limit > 100 {
			limit = 100
		} else if limit <= 0 {
			limit = 50
		}

		// 3. 呼叫底层引擎：执行极速 B+ 树索引扫描
		// 这里绝对不会产生 OFFSET 导致的深分页慢查询风暴
		messages, err := repository.Message.GetHistoryByCursor(roomID, cursorMsgID, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "拉取历史记录失败"})
			return
		}

		// 4. 组装下一次拉取的游标 (Next Cursor)
		var nextCursor int64 = 0
		if len(messages) > 0 {
			// 因为底层 SQL 是 ORDER BY id DESC，所以切片最后一个元素就是这批数据里最老的 (ID 最小的)
			nextCursor = messages[len(messages)-1].ID
		}

		// 告诉前端是否已经“触底” (没有更多历史记录了)
		hasMore := len(messages) == limit

		c.JSON(http.StatusOK, gin.H{
			"messages":    messages,
			"next_cursor": nextCursor,
			"has_more":    hasMore,
		})
	}
}
