package core

import (
	"encoding/json"
	"log"

	"lan-im-go/models"
)

// Hub 内存级路由引擎
type Hub struct {
	// 【核心双索引架构】：空间换时间
	// 1. 数据面扇出索引：RoomID -> 订阅了该房间的 Clients 集合
	rooms map[int64]map[*Client]bool
	// 2. 控制面狙击索引：UserID -> Client
	users map[int64]*Client
	// 调度管道 (全部复用 models.Message，实现全栈数据模型统一)
	Subscribe   chan *Subscription
	Unsubscribe chan *Subscription
	Broadcast   chan *models.Message
	DBBuffer    chan *models.Message
	Kick        chan int64
}

func NewHub() *Hub {
	return &Hub{
		rooms:       make(map[int64]map[*Client]bool),
		users:       make(map[int64]*Client),
		Subscribe:   make(chan *Subscription),
		Unsubscribe: make(chan *Subscription),
		Broadcast:   make(chan *models.Message, 1024), // 抵御突发消息洪峰
		DBBuffer:    make(chan *models.Message, 5000), // 异步落盘的高速缓冲
		Kick:        make(chan int64),
	}
}

func (h *Hub) Run() {
	go h.asyncDBWriter()
	for {
		select {
		case sub := <-h.Subscribe:
			h.users[sub.Client.UserID] = sub.Client
			for _, roomID := range sub.RoomIDs {
				if h.rooms[roomID] == nil {
					h.rooms[roomID] = make(map[*Client]bool)
				}
				h.rooms[roomID][sub.Client] = true
			}
			log.Printf("[Hub] 用户 %d 挂载了 %d 个路由节点", sub.Client.UserID, len(sub.RoomIDs))

		case unsub := <-h.Unsubscribe:
			for _, roomID := range unsub.RoomIDs {
				if clients, ok := h.rooms[roomID]; ok {
					delete(clients, unsub.Client)
					if len(clients) == 0 {
						delete(h.rooms, roomID)
					}
				}
			}
			// 断开连接时的彻底清理
			if _, ok := h.users[unsub.Client.UserID]; ok {
				delete(h.users, unsub.Client.UserID)
				close(unsub.Client.Send)
			}

		case msg := <-h.Broadcast:
			// 1. 无阻塞投递落盘缓冲
			select {
			case h.DBBuffer <- msg:
			default:
				log.Println("[Hub 致命警告] DBBuffer 已满！触发写保护丢包机制！")
			}

			// 2. 集中序列化 (千万别放进下面的 for 循环里)
			payload, err := json.Marshal(msg)
			if err != nil {
				continue
			}

			// 3. O(1) 极限路由转发
			if clients, ok := h.rooms[msg.RoomID]; ok {
				for client := range clients {
					// 过滤自己，防止消息重复回显
					if client.UserID == msg.SenderID {
						continue
					}
					select {
					case client.Send <- payload:
					default:
						// 强杀卡顿的幽灵连接
						close(client.Send)
						delete(clients, client)
						delete(h.users, client.UserID)
						client.Conn.Close()
					}
				}
			}

		case targetUserID := <-h.Kick:
			// O(1) 绝杀
			if client, ok := h.users[targetUserID]; ok {
				client.Conn.Close()
			}
		}
	}
}

func (h *Hub) asyncDBWriter() {
	for msg := range h.DBBuffer {
		// 注意这里改成了 msg.ID，因为在 models.Message 中我们使用的是 ID 而不是 MsgID
		log.Printf("[DB Writer] 正在异步落盘 RoomID:%d 的消息 MsgID:%d", msg.RoomID, msg.ID)
		// 严苛规范：在这里不要直接调 gorm.DB，要通过 repository 接口去落盘
		// repository.Message.SaveMessage(msg)
	}
}
