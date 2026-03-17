package core

import (
	"encoding/json"
	"log"
)

// 彻底精简后的统一消息体
type Message struct {
	MsgID    int64  `json:"msg_id"`
	SenderID int64  `json:"sender_id"`
	RoomID   int64  `json:"room_id"` // 万物皆房间 (你的 QID)
	Content  string `json:"content"`
}

// 订阅动作载体
type Subscription struct {
	Client  *Client
	RoomIDs []int64 // 该用户所属的所有房间 ID
}

type Hub struct {
	// 【核心重构】：不再维护 UserID -> Client
	// 改为维护 RoomID -> 订阅了该房间的 Clients 集合
	// 这就是一个标准的内存级 Pub/Sub 模型
	rooms map[int64]map[*Client]bool

	// 调度管道
	Subscribe   chan *Subscription
	Unsubscribe chan *Subscription
	Broadcast   chan *Message
	DBBuffer    chan *Message
	Register    chan *Client
	Unregister  chan *Client
}

func NewHub() *Hub {
	return &Hub{
		rooms:       make(map[int64]map[*Client]bool),
		Subscribe:   make(chan *Subscription),
		Unsubscribe: make(chan *Subscription),
		Broadcast:   make(chan *Message, 1024),
		DBBuffer:    make(chan *Message, 5000),
		Register:    make(chan *Client),
		Unregister:  make(chan *Client),
	}
}

func (h *Hub) Run() {
	go h.asyncDBWriter()

	for {
		select {
		case sub := <-h.Subscribe:
			// 用户上线，将其加入到他所属的所有房间中
			for _, roomID := range sub.RoomIDs {
				if h.rooms[roomID] == nil {
					h.rooms[roomID] = make(map[*Client]bool)
				}
				h.rooms[roomID][sub.Client] = true
			}
			log.Printf("[Hub] 用户 %d 订阅了 %d 个房间", sub.Client.UserID, len(sub.RoomIDs))

		case unsub := <-h.Unsubscribe:
			// 用户下线，从他所有的房间中移除该连接
			for _, roomID := range unsub.RoomIDs {
				if clients, ok := h.rooms[roomID]; ok {
					delete(clients, unsub.Client)
					// 内存优化：如果房间空了，回收 map
					if len(clients) == 0 {
						delete(h.rooms, roomID)
					}
				}
			}
			close(unsub.Client.Send)

		case msg := <-h.Broadcast:
			// 1. 扔进 DB 落盘缓冲池
			select {
			case h.DBBuffer <- msg:
			default:
				log.Println("[Hub 警告] DBBuffer 阻塞！")
			}

			// 2. O(1) 级别的极限路由转发
			// 直接找到该 RoomID 下所有的在线 Client 进行下发，再也不用去查 MySQL 找群成员了！
			payload, _ := json.Marshal(msg)
			if clients, ok := h.rooms[msg.RoomID]; ok {
				for client := range clients {
					// 不发给自己 (可选，取决于前端需不需要服务端 ACK 回显)
					if client.UserID == msg.SenderID {
						continue
					}
					select {
					case client.Send <- payload:
					default:
						// 强杀卡顿连接
						close(client.Send)
						delete(clients, client)
					}
				}
			}
		}
	}
}

func (h *Hub) asyncDBWriter() {
	for msg := range h.DBBuffer {
		log.Printf("[DB Writer] 写入 RoomID:%d 的消息 MsgID:%d", msg.RoomID, msg.MsgID)
		// db.Create(&models.Message{...})
	}
}
