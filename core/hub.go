package core

import (
	"encoding/json"
	"lan-im-go/models"
	"lan-im-go/repository"
	"log"
	"time"

	"github.com/bwmarrin/snowflake"
)

// 全局初始化雪花节点，只执行1次
var snowflakeNode *snowflake.Node

// 初始化函数，程序启动时执行一次
func init() {
	var err error
	snowflakeNode, err = snowflake.NewNode(1)
	if err != nil {
		log.Fatalf("雪花算法初始化失败: %v", err)
	}
}

type RoomAction struct {
	UserID int64
	RoomID int64
	Action string
}

// Hub 内存路由引擎
type Hub struct {
	// 核心双索引结构：空间换时间优化查询性能
	// 1. 房间索引：RoomID -> 订阅该房间的客户端集合
	rooms map[int64]map[*Client]bool
	// 2. 用户索引：UserID -> 客户端连接
	users map[int64]*Client
	// 调度通道：统一数据模型，提升系统兼容性
	Subscribe      chan *Subscription
	Unsubscribe    chan *Subscription
	Broadcast      chan *models.Message
	DBBuffer       chan *models.Message
	RoomActionChan chan *RoomAction
	Kick           chan int64
}

func NewHub() *Hub {
	return &Hub{
		rooms:          make(map[int64]map[*Client]bool), // 存储房间与客户端的关联关系
		users:          make(map[int64]*Client),          // 存储用户与客户端的关联关系
		Subscribe:      make(chan *Subscription),
		Unsubscribe:    make(chan *Subscription),
		Broadcast:      make(chan *models.Message, 1024),  // 缓冲通道，应对消息流量峰值
		DBBuffer:       make(chan *models.Message, 50000), // 异步持久化缓冲通道
		RoomActionChan: make(chan *RoomAction, 100),
		Kick:           make(chan int64),
	}
}

func (h *Hub) Run() {
	go h.asyncDBWriter()
	for {
		select {
		case sub := <-h.Subscribe:
			// 注册用户客户端
			h.users[sub.Client.UserID] = sub.Client
			// 订阅关联的群聊
			for _, roomID := range sub.RoomIDs {
				if h.rooms[roomID] == nil {
					h.rooms[roomID] = make(map[*Client]bool)
				}
				h.rooms[roomID][sub.Client] = true
			}
			log.Printf("[Hub] 用户 %d 订阅了 %d 个群聊", sub.Client.UserID, len(sub.RoomIDs))

		case unsub := <-h.Unsubscribe:
			// 取消群聊订阅
			for _, roomID := range unsub.RoomIDs {
				if clients, ok := h.rooms[roomID]; ok {
					delete(clients, unsub.Client)
					// 清理空群聊
					if len(clients) == 0 {
						delete(h.rooms, roomID)
					}
				}
			}
			// 彻底清理用户连接
			if _, ok := h.users[unsub.Client.UserID]; ok {
				delete(h.users, unsub.Client.UserID)
				close(unsub.Client.Send)
			}

		case msg := <-h.Broadcast:
			// 1. 非阻塞写入持久化缓冲
			msgID := snowflakeNode.Generate().Int64()
			msg.ID = msgID
			select {
			case h.DBBuffer <- msg:
			default:
				log.Println("[Hub 警告] 持久化缓冲已满，启动写保护机制")
			}

			// 2. 统一序列化，避免循环内重复序列化损耗性能
			payload, err := json.Marshal(msg)
			if err != nil {
				continue
			}

			// 3. 高效路由转发消息
			if clients, ok := h.rooms[msg.RoomID]; ok {
				for client := range clients {
					// 跳过消息发送者，避免重复接收
					// if client.UserID == msg.SenderID {    #测试
					// 	continue
					// }
					select {
					case client.Send <- payload:
					default:
						// 清理异常连接，释放资源
						close(client.Send)
						delete(clients, client)
						delete(h.users, client.UserID)
						client.Conn.Close()
					}
				}
			}

		case action := <-h.RoomActionChan:
			// 根据指令更新内存路由状态
			switch action.Action {
			case "join":
				// 用户加入群聊，更新订阅关系
				if client, ok := h.users[action.UserID]; ok {
					if h.rooms[action.RoomID] == nil {
						h.rooms[action.RoomID] = make(map[*Client]bool)
					}
					h.rooms[action.RoomID][client] = true
					log.Printf("[Hub 引擎] 路由更新：用户 %d 加入群聊 %d", action.UserID, action.RoomID)
				} else {
					log.Printf("[Hub 引擎] 用户 %d 处于离线状态，无需更新路由", action.UserID)
				}
			case "leave":
				// 用户退出/移出群聊，移除订阅关系
				if client, ok := h.users[action.UserID]; ok {
					if h.rooms[action.RoomID] != nil {
						delete(h.rooms[action.RoomID], client)
						log.Printf("[Hub 引擎] 路由更新：用户 %d 退出群聊 %d", action.UserID, action.RoomID)
					}
				}
			case "disband":
				// 解散群聊，清理内存路由
				if h.rooms[action.RoomID] != nil {
					delete(h.rooms, action.RoomID)
					log.Printf("[Hub 引擎] 路由清理：群聊 %d 已解散", action.RoomID)
				}
			default:
				log.Printf("[Hub 引擎 警告] 收到未知操作指令: %s", action.Action)
			}

		case targetUserID := <-h.Kick:
			// 强制关闭用户连接
			if client, ok := h.users[targetUserID]; ok {
				client.Conn.Close()
			}
		}
	}
}

// asyncDBWriter 异步消息持久化协程
func (h *Hub) asyncDBWriter() {
	// 批量插入缓存，每100条/500ms刷一次库
	const batchSize = 100
	const flushInterval = 500 * time.Millisecond

	var msgBatch []*models.Message
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		// 从通道取消息
		case msg, ok := <-h.DBBuffer:
			if !ok {
				// 通道关闭，刷最后一批数据
				h.flushBatch(msgBatch)
				return
			}

			msgBatch = append(msgBatch, msg)

			// 达到批量数量，立即写入
			if len(msgBatch) >= batchSize {
				h.flushBatch(msgBatch)
				// 清空切片
				msgBatch = make([]*models.Message, 0, batchSize)
			}

		// 定时写入，防止消息一直堆积
		case <-ticker.C:
			if len(msgBatch) > 0 {
				h.flushBatch(msgBatch)
				msgBatch = make([]*models.Message, 0, batchSize)
			}
		}
	}
}

// ✅ 批量写入数据库（性能提升 10~100 倍）
func (h *Hub) flushBatch(msgBatch []*models.Message) {
	if len(msgBatch) == 0 {
		return
	}

	// 批量插入，只执行1次SQL，而不是N次
	err := repository.Message.SaveMessageBatch(msgBatch)
	if err != nil {
		log.Printf("[持久化错误] 批量消息保存失败: %v", err)
	} else {
		log.Printf("[持久化成功] 批量写入 %d 条消息", len(msgBatch))
	}
}
