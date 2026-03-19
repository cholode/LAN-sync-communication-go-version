package core

import (
	"encoding/json"
	"github.com/bwmarrin/snowflake"
	"lan-im-go/models"
	"lan-im-go/repository"
	"log"
)

type RoomAction struct {
	UserID int64
	RoomID int64
	Action string
}

// Hub 内存级路由引擎
type Hub struct {
	// 【核心双索引架构】：空间换时间
	// 1. 数据面扇出索引：RoomID -> 订阅了该房间的 Clients 集合
	rooms map[int64]map[*Client]bool
	// 2. 控制面狙击索引：UserID -> Client
	users map[int64]*Client
	// 调度管道 (全部复用 models.Message，实现全栈数据模型统一)
	Subscribe      chan *Subscription
	Unsubscribe    chan *Subscription
	Broadcast      chan *models.Message
	DBBuffer       chan *models.Message
	RoomActionChan chan *RoomAction
	Kick           chan int64
}

func NewHub() *Hub {
	return &Hub{
		rooms:          make(map[int64]map[*Client]bool), //房间里有哪些人
		users:          make(map[int64]*Client),          //
		Subscribe:      make(chan *Subscription),
		Unsubscribe:    make(chan *Subscription),
		Broadcast:      make(chan *models.Message, 1024), // 抵御突发消息洪峰
		DBBuffer:       make(chan *models.Message, 5000), // 异步落盘的高速缓冲
		RoomActionChan: make(chan *RoomAction, 100),
		Kick:           make(chan int64),
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
			node, _ := snowflake.NewNode(1)
			msgID := node.Generate().Int64()
			msg.ID = msgID
			select {
			case h.DBBuffer <- msg:

				//log.Printf("雪花id是-------------%d \n", msgID)
			default:
				log.Println("[Hub 致命警告] DBBuffer 已满！触发写保护丢包机制！")
			}

			//2. 集中序列化 (千万别放进下面的 for 循环里)
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

		case action := <-h.RoomActionChan:
			// ⚡ 核心防线：必须根据 HTTP 接口透传过来的 Action 指令，执行对应的内存拓扑修改！
			switch action.Action {
			case "join":
				if client, ok := h.users[action.UserID]; ok {
					if h.rooms[action.RoomID] == nil {
						h.rooms[action.RoomID] = make(map[*Client]bool)
					}
					h.rooms[action.RoomID][client] = true
					log.Printf("[Hub 引擎] 内存路由热更新：UID:%d 动态接入 Room:%d", action.UserID, action.RoomID)
				} else {
					log.Printf("[Hub 引擎] UID:%d 当前离线，无需热挂载路由", action.UserID)
				}
			case "leave":
				// 🔪 单兵剥离：踢人或退群
				if client, ok := h.users[action.UserID]; ok {
					if h.rooms[action.RoomID] != nil {
						// 从该群的活跃订阅字典中，精准删除这个 client 指针
						delete(h.rooms[action.RoomID], client)
						log.Printf("[Hub 引擎] 路由已斩断：UID:%d 被物理剥离 Room:%d", action.UserID, action.RoomID)
						// 可选：你甚至可以直接通过 client.Send 给他发一条系统消息："你已被移出群聊"
					}
				}
			case "disband":
				// 💥 级联毁灭：群主解散群聊
				if h.rooms[action.RoomID] != nil {
					// 架构师的干净利落：直接把整个群的 map 从内存中连根拔起，交给 Go GC (垃圾回收) 处理！
					delete(h.rooms, action.RoomID)
					log.Printf("[Hub 引擎] 频段覆灭：Room:%d 的内存路由树已被彻底摧毁！", action.RoomID)
				}
			default:
				log.Printf("[Hub 引擎 致命警告] 收到未知的内存操纵指令: %s", action.Action)
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
		go func(m *models.Message) {
			// 直接接收接口返回的 error！绝不能带 .Error！
			if err := repository.Message.SaveMessage(m); err != nil {
				log.Printf("[持久化致命错误] 消息落盘失败, RoomID: %d, SenderID: %d, Err: %v", m.RoomID, m.SenderID, err)
				// 注意：这里是异步协程，千万不要在这里 c.JSON() 或 return，没有意义
			} else {
				log.Printf("[持久化成功] 消息已安全落入物理硬盘 (MsgID: %d)", m.ID)
			}
		}(msg)
		log.Printf("[DB Writer] 正在异步落盘 RoomID:%d 的消息 MsgID:%d", msg.RoomID, msg.ID)
		// 严苛规范：在这里不要直接调 gorm.DB，要通过 repository 接口去落盘
		// repository.Message.SaveMessage(msg)
	}
}
