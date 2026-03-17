package repository

import (
	"gorm.io/gorm"
	"lan-im-go/models"
)

type messageRepoImpl struct {
	db *gorm.DB
}

func NewMessageRepoImpl(db *gorm.DB) MessageRepository {
	return &messageRepoImpl{db: db}
}

func (r *messageRepoImpl) SaveMessage(msg *models.Message) error {
	// 被 Hub 的 asyncDBWriter 疯狂调用的落盘接口，要求极致的写性能
	return r.db.Create(msg).Error
}

// GetHistoryByCursor 架构师的骄傲：真正的触顶无感加载游标分页
func (r *messageRepoImpl) GetHistoryByCursor(roomID int64, cursorMsgID int64, limit int) ([]*models.Message, error) {
	var messages []*models.Message
	// 1. 第一层过滤：必须命中 room_id 索引
	query := r.db.Where("room_id = ?", roomID)
	// 2. 第二层过滤：游标判定
	// 如果前端传来的 cursorMsgID > 0，说明是向上翻页拉取更老的历史记录
	// 如果是 0，说明是刚进群，拉取最新的 limit 条消息
	if cursorMsgID > 0 {
		query = query.Where("id < ?", cursorMsgID)
	}
	// 3. 树遍历与截断：利用 B+ 树的天然有序性，直接从游标位置倒序读取 limit 条
	// 严苛要求：这个 SQL 完美契合了我们在 models 中建立的 `index:idx_room_id` 组合索引
	err := query.Order("id DESC").Limit(limit).Find(&messages).Error
	return messages, err
}

// SoftDeleteUserMessagesInRoom B端管控业务：精确抹除某人在某群的全部痕迹
func (r *messageRepoImpl) SoftDeleteUserMessagesInRoom(roomID int64, userID int64) error {
	// 面试官防杠细节：
	// 为什么不执行物理 DELETE？因为如果物理删除了中间的消息，游标分页的连续性虽然不受影响，
	// 但可能会导致 B+ 树页合并，在极高并发的 IM 场景中引发树锁。
	// 采用软删（UPDATE deleted_at），不仅能保留公安等部门备查的日志，还能最大程度保证数据库 IO 的平滑。
	return r.db.Model(&models.Message{}).
		Where("room_id = ? AND sender_id = ?", roomID, userID).
		Update("deleted_at", gorm.Expr("NOW()")).Error
}
